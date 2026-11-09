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

func openBulkQuoteTestDB(t *testing.T) (*Service, *gorm.DB, Client) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&subjects.Subject{}, &Client{}, &Appointment{}, &Quotation{}, &ExamPrepaidSlot{}, &linkedTestOrgRow{}))
	require.NoError(t, db.Create(&linkedTestOrgRow{ID: 1, Currency: "AED", UsdAedRate: 3.6725}).Error)
	client := Client{Name: "Pioneer LLC", ClientType: "Corporate", Email: "corp@test.example"}
	require.NoError(t, db.Create(&client).Error)
	return &Service{db: db}, db, client
}

func createBulkQuote(t *testing.T, svc *Service, clientID uint, qty int, amount float64) Quotation {
	t.Helper()
	q := Quotation{ClientID: clientID, Title: "Corporate polygraph test slot", LineQuantity: qty, Amount: amount, Currency: "AED"}
	require.NoError(t, svc.CreateQuotation(&q))
	return q
}

// bookSlotExam mirrors CreateAppointment's slot path without availability checks.
func bookSlotExam(t *testing.T, svc *Service, db *gorm.DB, clientID uint, fee float64) Appointment {
	t.Helper()
	appt := Appointment{ClientID: clientID, SubjectID: 1, ExaminerID: 1, ScheduledAt: time.Now(), Duration: 150, ExamFee: fee, FeeCurrency: "AED"}
	require.NoError(t, db.Create(&appt).Error)
	_, err := svc.ApplyPrepaidSlotToAppointment(strconv.FormatUint(uint64(appt.ID), 10))
	require.NoError(t, err)
	require.NoError(t, db.First(&appt, appt.ID).Error)
	return appt
}

func countSlots(t *testing.T, db *gorm.DB, quoteID uint, status string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ? AND status = ?", quoteID, status).Count(&n).Error)
	return n
}

func TestCorporateQuoteCreatesSlotsBeforePayment(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 5, 15015)
	require.Equal(t, int64(5), countSlots(t, db, q.ID, "available"))

	summary, err := svc.GetClientPrepaidSlotSummary(strconv.FormatUint(uint64(client.ID), 10))
	require.NoError(t, err)
	require.Equal(t, 5, summary.Available)
	require.Equal(t, 5, summary.AwaitingPayment)
}

func TestIndividualQuoteDoesNotCreateSlots(t *testing.T) {
	svc, db, _ := openBulkQuoteTestDB(t)
	person := Client{Name: "Jane", ClientType: "Individual", Email: "jane@test.example"}
	require.NoError(t, db.Create(&person).Error)
	q := createBulkQuote(t, svc, person.ID, 1, 3003)
	require.Equal(t, int64(0), countSlots(t, db, q.ID, "available"))
}

func TestSlotBookingFollowsBulkInvoicePayment(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 2, 6006)

	appt := bookSlotExam(t, svc, db, client.ID, 3003)
	require.NotNil(t, appt.PrepaidSlotID)
	require.Equal(t, "Unpaid", appt.PaymentStatus)
	require.Equal(t, float64(0), appt.CollectedAmount)

	var sessionInvoices int64
	require.NoError(t, db.Model(&Quotation{}).Where("appointment_id = ?", appt.ID).Count(&sessionInvoices).Error)
	require.Equal(t, int64(0), sessionInvoices, "slot booking must not get its own invoice")

	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(q.ID), 10), 6006))
	require.NoError(t, db.First(&appt, appt.ID).Error)
	require.Equal(t, "Paid", appt.PaymentStatus)
	require.Equal(t, float64(3003), appt.CollectedAmount)

	// Paying must not mint extra slots: still 2 in total.
	require.Equal(t, int64(1), countSlots(t, db, q.ID, "available"))
	require.Equal(t, int64(1), countSlots(t, db, q.ID, "consumed"))
}

func TestLedgerDoesNotDoubleBillSlotBookings(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 2, 6006)
	bookSlotExam(t, svc, db, client.ID, 3003)
	bookSlotExam(t, svc, db, client.ID, 3003)
	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(q.ID), 10), 6006))

	entries, summary, err := svc.buildBillingLedger(strconv.FormatUint(uint64(client.ID), 10))
	require.NoError(t, err)
	require.Equal(t, float64(6006), summary.TotalBilled)
	require.Equal(t, float64(6006), summary.TotalPaid)
	covered := 0
	for _, e := range entries {
		if e.CoveredBy != "" {
			covered++
			require.Equal(t, float64(0), e.TotalAmount)
		}
	}
	require.Equal(t, 2, covered)
}

func TestApplySlotRemovesSessionInvoice(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	createBulkQuote(t, svc, client.ID, 1, 3003)

	appt := Appointment{ClientID: client.ID, SubjectID: 1, ExaminerID: 1, ScheduledAt: time.Now(), Duration: 150,
		ExamFee: 2939, FeeCurrency: "AED", CollectedAmount: 2939, PaymentStatus: "Paid"}
	require.NoError(t, db.Create(&appt).Error)
	aid := appt.ID
	session := Quotation{ClientID: client.ID, AppointmentID: &aid, Code: "009326", Title: "Session", Amount: 2939, CollectedAmount: 2939, Status: "Completed", Currency: "AED"}
	require.NoError(t, db.Create(&session).Error)

	_, err := svc.ApplyPrepaidSlotToAppointment(strconv.FormatUint(uint64(appt.ID), 10))
	require.NoError(t, err)

	var alive int64
	require.NoError(t, db.Model(&Quotation{}).Where("id = ?", session.ID).Count(&alive).Error)
	require.Equal(t, int64(0), alive)

	// No free slots left.
	other := Appointment{ClientID: client.ID, SubjectID: 1, ExaminerID: 1, ScheduledAt: time.Now(), Duration: 150, ExamFee: 3003}
	require.NoError(t, db.Create(&other).Error)
	_, err = svc.ApplyPrepaidSlotToAppointment(strconv.FormatUint(uint64(other.ID), 10))
	require.Error(t, err)
}

func TestApplySlotRefusesInvoiceWithRealPayment(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	createBulkQuote(t, svc, client.ID, 1, 3003)
	appt := Appointment{ClientID: client.ID, SubjectID: 1, ExaminerID: 1, ScheduledAt: time.Now(), Duration: 150, ExamFee: 3003}
	require.NoError(t, db.Create(&appt).Error)
	aid := appt.ID
	session := Quotation{ClientID: client.ID, AppointmentID: &aid, Code: "009400", Title: "Session", Amount: 3003, CollectedAmount: 3003, Status: "Completed", Currency: "AED",
		PaymentHistoryJSON: `[{"paid_at":"2026-11-09T00:00:00Z","amount":3003,"total_charged":3003,"method":"stripe"}]`}
	require.NoError(t, db.Create(&session).Error)

	_, err := svc.ApplyPrepaidSlotToAppointment(strconv.FormatUint(uint64(appt.ID), 10))
	require.Error(t, err)
}

func TestBulkQuoteQuantityAndDeleteGuards(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 3, 9009)
	id := strconv.FormatUint(uint64(q.ID), 10)
	bookSlotExam(t, svc, db, client.ID, 3003)

	_, err := svc.UpdateQuotationPricing(id, map[string]interface{}{"line_quantity": float64(5)})
	require.NoError(t, err)
	require.Equal(t, int64(4), countSlots(t, db, q.ID, "available"))

	_, err = svc.UpdateQuotationPricing(id, map[string]interface{}{"line_quantity": float64(2)})
	require.NoError(t, err)
	require.Equal(t, int64(1), countSlots(t, db, q.ID, "available"))

	_, err = svc.UpdateQuotationPricing(id, map[string]interface{}{"line_quantity": float64(0.5)})
	require.Error(t, err)

	require.Error(t, svc.DeleteQuotation(id), "cannot delete a bulk invoice with booked exams")
}

func TestDepositMakesSlotExamsPartial(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 2, 6000)
	appt := bookSlotExam(t, svc, db, client.ID, 3000)

	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(q.ID), 10), 1800)) // 30%
	require.NoError(t, db.First(&appt, appt.ID).Error)
	require.Equal(t, "Partial", appt.PaymentStatus)
	require.Equal(t, float64(900), appt.CollectedAmount)

	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(q.ID), 10), 4200))
	require.NoError(t, db.First(&appt, appt.ID).Error)
	require.Equal(t, "Paid", appt.PaymentStatus)
}

func TestCancellingSlotExamReturnsSlot(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := createBulkQuote(t, svc, client.ID, 1, 3003)
	appt := bookSlotExam(t, svc, db, client.ID, 3003)
	require.Equal(t, int64(0), countSlots(t, db, q.ID, "available"))

	require.NoError(t, svc.UpdateStatus(strconv.FormatUint(uint64(appt.ID), 10), "cancelled"))
	require.Equal(t, int64(1), countSlots(t, db, q.ID, "available"))
	require.NoError(t, db.First(&appt, appt.ID).Error)
	require.Nil(t, appt.PrepaidSlotID)
}

func TestRevokeSlotRepricesInvoice(t *testing.T) {
	svc, db, client := openBulkQuoteTestDB(t)
	q := Quotation{ClientID: client.ID, Title: "Pack", LineQuantity: 5, SubtotalAmount: 14300, VatRate: 5, VatAmount: 715, Amount: 15015, Currency: "AED"}
	require.NoError(t, svc.CreateQuotation(&q))
	id := strconv.FormatUint(uint64(q.ID), 10)

	got, err := svc.RevokeSlotFromQuotation(id)
	require.NoError(t, err)
	require.Equal(t, 4, got.LineQuantity)
	require.Equal(t, float64(11440), got.SubtotalAmount)
	require.Equal(t, float64(12012), got.Amount) // 4 x 3,003
	require.Equal(t, int64(4), countSlots(t, db, q.ID, "available"))

	// Paid 12,012 already: revoking another would drop the total below what was paid.
	require.NoError(t, svc.CollectQuotationPayment(id, 12012))
	_, err = svc.RevokeSlotFromQuotation(id)
	require.Error(t, err)
}
