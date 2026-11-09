package appointments

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRepairPioneervisionDoubleBilling(t *testing.T) {
	svc, db, _ := openBulkQuoteTestDB(t)
	require.NoError(t, db.Create(&Client{ID: 38, Name: "Pioneervision", ClientType: "Corporate", Email: "p@test.example"}).Error)
	require.NoError(t, db.Create(&Quotation{ID: 319, ClientID: 38, Code: "009319", Title: "Bulk", LineQuantity: 5, Amount: 15015, CollectedAmount: 15015, Status: "Completed", Currency: "AED"}).Error)
	require.NoError(t, db.Create(&Quotation{ID: 332, ClientID: 38, Code: "009332", Title: "Bulk", LineQuantity: 1, Amount: 3003, CollectedAmount: 3003, Status: "Completed", Currency: "AED"}).Error)
	for i := uint(0); i < 6; i++ {
		src := uint(319)
		if i == 5 {
			src = 332
		}
		require.NoError(t, db.Create(&ExamPrepaidSlot{ID: 33 + i, ClientID: 38, SourceQuotationID: src, Status: "available"}).Error)
		apptID := 299 + i
		require.NoError(t, db.Create(&Appointment{ID: apptID, ClientID: 38, SubjectID: 1, ExaminerID: 1, ScheduledAt: time.Now(), ExamFee: 2939, CollectedAmount: 2939, PaymentStatus: "Paid", Status: "completed"}).Error)
		qid := uint(326) + i
		if i == 5 {
			qid = 334
		}
		require.NoError(t, db.Create(&Quotation{ID: qid, ClientID: 38, AppointmentID: &apptID, Code: fmt.Sprintf("s%d", qid), Title: "s", Amount: 2939, CollectedAmount: 2939, Status: "Completed", Currency: "AED"}).Error)
	}

	applied, err := RepairPioneervisionDoubleBilling(db)
	require.NoError(t, err)
	require.True(t, applied)

	var free, live int64
	db.Model(&ExamPrepaidSlot{}).Where("client_id = 38 AND status = 'available'").Count(&free)
	db.Model(&Quotation{}).Where("id IN ?", []uint{326, 327, 328, 329, 330, 334}).Count(&live)
	require.Equal(t, int64(0), free)
	require.Equal(t, int64(0), live)

	_, summary, err := svc.buildBillingLedger("38")
	require.NoError(t, err)
	require.Equal(t, float64(18018), summary.TotalBilled)
	require.Equal(t, float64(18018), summary.TotalPaid)

	// Second boot: no-op.
	applied, err = RepairPioneervisionDoubleBilling(db)
	require.NoError(t, err)
	require.False(t, applied)
}
