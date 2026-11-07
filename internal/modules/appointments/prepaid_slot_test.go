package appointments

import (
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testOrgRow mirrors organization_settings without importing settings (avoids import cycle).
type testOrgRow struct {
	ID         uint    `gorm:"primarykey"`
	Name       string  `gorm:"size:255;not null"`
	Currency   string  `gorm:"size:10;default:'AED'"`
	UsdAedRate float64 `gorm:"default:3.6725"`
}

func (testOrgRow) TableName() string { return "organization_settings" }

func openPrepaidTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Client{},
		&Quotation{},
		&ExamPrepaidSlot{},
		&testOrgRow{},
	))
	require.NoError(t, db.Create(&testOrgRow{
		ID:       1,
		Name:     "Test Lab",
		Currency: "AED",
	}).Error)
	return db
}

func seedCorporateClient(t *testing.T, db *gorm.DB) Client {
	t.Helper()
	c := Client{Name: "Acme Corp", ClientType: "Corporate", Email: "corp@test.example"}
	require.NoError(t, db.Create(&c).Error)
	return c
}

func seedStandaloneQuote(t *testing.T, db *gorm.DB, clientID uint, qty int, amount float64, collected float64, status string) Quotation {
	t.Helper()
	q := Quotation{
		ClientID:        clientID,
		Code:            "900001",
		Title:           "Screening exam",
		LineQuantity:    qty,
		Amount:          amount,
		SubtotalAmount:  amount,
		CollectedAmount: collected,
		Status:          status,
		Currency:        "AED",
	}
	require.NoError(t, db.Create(&q).Error)
	return q
}

func TestEnsureSlotsForPaidQuotation_MintsOnFullPayment(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	quote := seedStandaloneQuote(t, db, client.ID, 10, 1000, 1000, "Completed")

	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &quote))

	var count int64
	require.NoError(t, db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&count).Error)
	require.Equal(t, int64(10), count)
}

func TestEnsureSlotsForPaidQuotation_Idempotent(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	quote := seedStandaloneQuote(t, db, client.ID, 3, 300, 300, "Completed")

	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &quote))
	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &quote))

	var count int64
	require.NoError(t, db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&count).Error)
	require.Equal(t, int64(3), count)
}

func TestEnsureSlotsForPaidQuotation_NoMintOnPartial(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	quote := seedStandaloneQuote(t, db, client.ID, 5, 500, 200, "Partial")

	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &quote))

	var count int64
	require.NoError(t, db.Model(&ExamPrepaidSlot{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestCollectQuotationPayment_UsesVATInclusiveTotal(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	// Legacy row: amount field is a deposit; subtotal + VAT is the real total.
	q := Quotation{
		ClientID:        client.ID,
		Code:            "900002",
		Title:           "Bulk screening",
		LineQuantity:    6,
		Amount:          3087,
		SubtotalAmount:  8820,
		VatRate:         5,
		VatAmount:       441,
		CollectedAmount: 3087,
		Status:          "Completed",
		Currency:        "AED",
	}
	require.NoError(t, db.Create(&q).Error)

	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(q.ID), 10), 6174))

	var updated Quotation
	require.NoError(t, db.First(&updated, q.ID).Error)
	require.Equal(t, float64(9261), updated.CollectedAmount)
	require.Equal(t, "Completed", updated.Status)
	require.Equal(t, float64(9261), updated.Amount)
}

func TestCollectQuotationPayment_MintsSlotsWhenFullyPaid(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	quote := seedStandaloneQuote(t, db, client.ID, 4, 400, 0, "Pending")

	require.NoError(t, svc.CollectQuotationPayment(strconv.FormatUint(uint64(quote.ID), 10), 400))

	var count int64
	require.NoError(t, db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&count).Error)
	require.Equal(t, int64(4), count)
}

func TestRedeemPrepaidSlotForBooking_ReturnsFirstAvailable(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	quote := seedStandaloneQuote(t, db, client.ID, 2, 200, 200, "Completed")
	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &quote))

	slot, note, err := svc.redeemPrepaidSlotForBooking(db, client.ID)
	require.NoError(t, err)
	require.NotNil(t, slot)
	require.Contains(t, note, "slot 1 of 2")
	require.Equal(t, "available", slot.Status)
}

func TestRedeemPrepaidSlotForBooking_NoSlotsReturnsNil(t *testing.T) {
	db := openPrepaidTestDB(t)
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)

	slot, note, err := svc.redeemPrepaidSlotForBooking(db, client.ID)
	require.NoError(t, err)
	require.Nil(t, slot)
	require.Empty(t, note)
}

func TestDeleteAppointment_ReleasesPrepaidSlotAndVoidsSessionInvoice(t *testing.T) {
	db := openPrepaidTestDB(t)
	require.NoError(t, db.AutoMigrate(&Appointment{}))
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	bulk := seedStandaloneQuote(t, db, client.ID, 2, 200, 200, "Completed")
	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &bulk))

	// Simulate a booking that redeemed the first slot.
	var slot ExamPrepaidSlot
	require.NoError(t, db.Order("id ASC").First(&slot).Error)
	appt := Appointment{ClientID: client.ID, PrepaidSlotID: &slot.ID, PaymentStatus: "Paid"}
	require.NoError(t, db.Create(&appt).Error)
	require.NoError(t, db.Model(&slot).Updates(map[string]interface{}{"status": "consumed", "appointment_id": appt.ID}).Error)
	session := Quotation{ClientID: client.ID, AppointmentID: &appt.ID, Amount: 100, CollectedAmount: 100, Status: "Completed"}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, svc.DeleteAppointment(strconv.FormatUint(uint64(appt.ID), 10)))

	sum, err := svc.GetClientPrepaidSlotSummary(strconv.FormatUint(uint64(client.ID), 10))
	require.NoError(t, err)
	require.Equal(t, 2, sum.Available)
	require.Equal(t, 0, sum.Consumed)

	var sessionCount int64
	require.NoError(t, db.Model(&Quotation{}).Where("id = ?", session.ID).Count(&sessionCount).Error)
	require.Equal(t, int64(0), sessionCount)
	var bulkCount int64
	require.NoError(t, db.Model(&Quotation{}).Where("id = ?", bulk.ID).Count(&bulkCount).Error)
	require.Equal(t, int64(1), bulkCount)
}

func TestRepairOrphanedPrepaidSlots_RestoresSlotsAndIsIdempotent(t *testing.T) {
	db := openPrepaidTestDB(t)
	require.NoError(t, db.AutoMigrate(&Appointment{}))
	svc := &Service{db: db}
	client := seedCorporateClient(t, db)
	bulk := seedStandaloneQuote(t, db, client.ID, 2, 200, 200, "Completed")
	require.NoError(t, svc.ensureSlotsForPaidQuotation(db, &bulk))

	var slots []ExamPrepaidSlot
	require.NoError(t, db.Order("id ASC").Find(&slots).Error)

	// Slot 0: consumed by a deleted booking (the bug). Slot 1: consumed by a live booking.
	dead := Appointment{ClientID: client.ID, PrepaidSlotID: &slots[0].ID}
	live := Appointment{ClientID: client.ID, PrepaidSlotID: &slots[1].ID}
	require.NoError(t, db.Create(&dead).Error)
	require.NoError(t, db.Create(&live).Error)
	require.NoError(t, db.Model(&slots[0]).Updates(map[string]interface{}{"status": "consumed", "appointment_id": dead.ID}).Error)
	require.NoError(t, db.Model(&slots[1]).Updates(map[string]interface{}{"status": "consumed", "appointment_id": live.ID}).Error)
	deadInv := Quotation{Code: "900101", ClientID: client.ID, AppointmentID: &dead.ID, Amount: 100, CollectedAmount: 100, Status: "Completed"}
	liveInv := Quotation{Code: "900102", ClientID: client.ID, AppointmentID: &live.ID, Amount: 100, CollectedAmount: 100, Status: "Completed"}
	require.NoError(t, db.Create(&deadInv).Error)
	require.NoError(t, db.Create(&liveInv).Error)
	require.NoError(t, db.Delete(&dead).Error) // old buggy delete: soft-delete only

	released, voided, err := RepairOrphanedPrepaidSlots(db)
	require.NoError(t, err)
	require.Equal(t, int64(1), released)
	require.Equal(t, int64(1), voided)

	sum, err := svc.GetClientPrepaidSlotSummary(strconv.FormatUint(uint64(client.ID), 10))
	require.NoError(t, err)
	require.Equal(t, 1, sum.Available)
	require.Equal(t, 1, sum.Consumed)

	var liveInvCount int64
	require.NoError(t, db.Model(&Quotation{}).Where("id = ?", liveInv.ID).Count(&liveInvCount).Error)
	require.Equal(t, int64(1), liveInvCount)

	released, voided, err = RepairOrphanedPrepaidSlots(db)
	require.NoError(t, err)
	require.Zero(t, released)
	require.Zero(t, voided)
}
