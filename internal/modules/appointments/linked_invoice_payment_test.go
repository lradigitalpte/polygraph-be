package appointments

import (
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/modules/subjects"
)

// linkedTestOrgRow carries every column money.LoadRates selects so the org stays AED.
type linkedTestOrgRow struct {
	ID         uint   `gorm:"primarykey"`
	Currency   string `gorm:"size:10"`
	UsdAedRate float64
	UsdGbpRate float64
	UsdEurRate float64
}

func (linkedTestOrgRow) TableName() string { return "organization_settings" }

// seedRepricedBookingInvoice mirrors a booking marked Paid at the single-exam fee
// (3,087 incl. VAT) whose invoice was later repriced to 3 × 2,940 + 5% VAT = 9,261.
func seedRepricedBookingInvoice(t *testing.T) (*Service, *gorm.DB, Appointment, Quotation) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&subjects.Subject{}, &Client{}, &Appointment{}, &Quotation{}, &ExamPrepaidSlot{}, &linkedTestOrgRow{}))
	require.NoError(t, db.Create(&linkedTestOrgRow{ID: 1, Currency: "AED", UsdAedRate: 3.6725}).Error)

	client := Client{Name: "Acme FZCO", Email: "acme@test.example"}
	require.NoError(t, db.Create(&client).Error)

	appt := Appointment{
		ClientID:        client.ID,
		SubjectID:       1,
		ExaminerID:      1,
		ScheduledAt:     time.Date(2026, 10, 23, 5, 0, 0, 0, time.UTC),
		Duration:        150,
		Notes:           "Employee Test",
		ExamFee:         3087,
		FeeCurrency:     "AED",
		CollectedAmount: 3087,
		PaymentStatus:   "Paid",
	}
	require.NoError(t, db.Create(&appt).Error)

	apptID := appt.ID
	quote := Quotation{
		ClientID:        client.ID,
		AppointmentID:   &apptID,
		Code:            "009294",
		Title:           "Specific Issue Testing",
		LineQuantity:    3,
		Amount:          3087,
		SubtotalAmount:  8820,
		VatRate:         5,
		VatAmount:       441,
		CollectedAmount: 3087,
		Status:          "Partial",
		Currency:        "AED",
	}
	require.NoError(t, db.Create(&quote).Error)
	return &Service{db: db}, db, appt, quote
}

func ledgerEntryForQuote(t *testing.T, svc *Service, quoteID uint) AccountLedgerEntry {
	t.Helper()
	entries, _, err := svc.buildBillingLedger("")
	require.NoError(t, err)
	for _, e := range entries {
		if e.QuotationID != nil && *e.QuotationID == quoteID {
			return e
		}
	}
	t.Fatalf("no ledger entry for quotation %d", quoteID)
	return AccountLedgerEntry{}
}

func TestLedger_LinkedInvoiceUsesInvoiceTotals(t *testing.T) {
	svc, db, _, quote := seedRepricedBookingInvoice(t)

	entry := ledgerEntryForQuote(t, svc, quote.ID)
	require.Equal(t, 9261.0, entry.TotalAmount)
	require.Equal(t, 3087.0, entry.PaidAmount)
	require.Equal(t, 6174.0, entry.BalanceDue)
	require.Equal(t, "Partial", entry.Status)

	// Loading the ledger must not overwrite the invoice from the appointment.
	var stored Quotation
	require.NoError(t, db.First(&stored, quote.ID).Error)
	require.Equal(t, "Partial", stored.Status)
	require.Equal(t, 3087.0, stored.CollectedAmount)
}

func TestCollectAppointmentPayment_LinkedInvoiceRecordsFullBalance(t *testing.T) {
	svc, db, appt, quote := seedRepricedBookingInvoice(t)

	_, err := svc.CollectAppointmentPayment(strconv.FormatUint(uint64(appt.ID), 10), 6174)
	require.NoError(t, err)

	var stored Quotation
	require.NoError(t, db.First(&stored, quote.ID).Error)
	require.Equal(t, 9261.0, stored.CollectedAmount)
	require.Equal(t, "Completed", stored.Status)
	require.Len(t, parseQuotationPaymentHistory(stored.PaymentHistoryJSON), 1)

	entry := ledgerEntryForQuote(t, svc, quote.ID)
	require.Equal(t, 9261.0, entry.PaidAmount)
	require.Equal(t, 0.0, entry.BalanceDue)
	require.Equal(t, "Paid", entry.Status)

	require.NoError(t, db.First(&stored, quote.ID).Error)
	require.Equal(t, 9261.0, stored.CollectedAmount)
}

func TestUpdatePayment_PaidSettlesLinkedInvoice(t *testing.T) {
	svc, db, appt, quote := seedRepricedBookingInvoice(t)

	require.NoError(t, svc.UpdatePayment(strconv.FormatUint(uint64(appt.ID), 10), "Paid", nil))

	var stored Quotation
	require.NoError(t, db.First(&stored, quote.ID).Error)
	require.Equal(t, 9261.0, stored.CollectedAmount)
	require.Equal(t, "Completed", stored.Status)
}
