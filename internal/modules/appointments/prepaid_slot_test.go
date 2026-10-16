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
