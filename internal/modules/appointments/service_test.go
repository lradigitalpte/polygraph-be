package appointments

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/modules/auth"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/rbac"
	"my-app/internal/modules/subjects"
)

// setupTestDB creates an in-memory SQLite database and runs migrations for testing
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	// Use unique memory string to avoid shared state across tests
	dbName := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	// Migrate the schemas
	err = db.AutoMigrate(&rbac.Role{}, &auth.User{}, &subjects.Subject{}, &availability.Block{}, &Client{}, &Appointment{}, &Quotation{})
	require.NoError(t, err)

	return db
}

func seedBookableExaminer(t *testing.T, db *gorm.DB) auth.User {
	t.Helper()

	role := rbac.Role{Name: "Examiner", Description: "Examiner"}
	require.NoError(t, db.Create(&role).Error)

	user := auth.User{
		Name:   "Examiner One",
		Email:  fmt.Sprintf("examiner-%d@test.com", time.Now().UnixNano()),
		RoleID: role.ID,
		Status: "active",
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// bookableTime returns a future timestamp that is not a Sunday, since
// validateAppointment rejects Sunday bookings. Without this, the suite fails
// whenever "now + 24h" happens to land on a Sunday (e.g. when run on a Saturday).
func bookableTime() time.Time {
	t := time.Now().Add(24 * time.Hour)
	for t.UTC().Weekday() == time.Sunday {
		t = t.Add(24 * time.Hour)
	}
	return t
}

func seedSubject(t *testing.T, db *gorm.DB) subjects.Subject {
	t.Helper()

	subject := subjects.Subject{
		FirstName: "Jane",
		LastName:  "Doe",
		IDNumber:  fmt.Sprintf("ID-%d", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(&subject).Error)
	return subject
}

func TestService_CreateClient(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	client := &Client{
		Name:  "Test Client",
		Email: "test@example.com",
		Phone: "1234567890",
	}

	err := s.CreateClient(client)
	assert.NoError(t, err)
	assert.NotZero(t, client.ID)

	var found Client
	err = db.First(&found, client.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, "Test Client", found.Name)
	assert.Equal(t, "test@example.com", found.Email)
}

func TestService_GetAllClients(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	client1 := &Client{Name: "Client A", Email: "a@test.com"}
	client2 := &Client{Name: "Client B", Email: "b@test.com"}
	require.NoError(t, s.CreateClient(client1))
	require.NoError(t, s.CreateClient(client2))

	clients, err := s.GetAllClients()
	assert.NoError(t, err)
	assert.Len(t, clients, 2)
}

func TestService_CreateAndGetAllAppointments(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	client := &Client{Name: "Appt Client", Email: "appt@test.com"}
	require.NoError(t, s.CreateClient(client))
	examiner := seedBookableExaminer(t, db)
	subject := seedSubject(t, db)

	scheduledAt := bookableTime()
	app := &Appointment{
		ClientID:    client.ID,
		SubjectID:   subject.ID,
		ExaminerID:  examiner.ID,
		ScheduledAt: scheduledAt,
		Duration:    120,
		Status:      "pending",
	}

	err := s.CreateAppointment(app)
	assert.NoError(t, err)
	assert.NotZero(t, app.ID)

	apps, err := s.GetAllAppointments()
	assert.NoError(t, err)
	assert.Len(t, apps, 1)
	assert.Equal(t, client.ID, apps[0].ClientID)
	assert.Equal(t, "Appt Client", apps[0].Client.Name) // Test Preload
	assert.Equal(t, "pending", apps[0].Status)
}

func TestService_CreateAppointmentAllowsPastScheduledAt(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	client := &Client{Name: "Backdate Client", Email: "backdate@test.com"}
	require.NoError(t, s.CreateClient(client))
	examiner := seedBookableExaminer(t, db)
	subject := seedSubject(t, db)

	// Yesterday at 10:00 Dubai time — skip Sundays the same way bookableTime does.
	loc, err := time.LoadLocation("Asia/Dubai")
	require.NoError(t, err)
	nowLocal := time.Now().In(loc)
	past := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 10, 0, 0, 0, loc).Add(-24 * time.Hour)
	for past.Weekday() == time.Sunday {
		past = past.Add(-24 * time.Hour)
	}

	app := &Appointment{
		ClientID:    client.ID,
		SubjectID:   subject.ID,
		ExaminerID:  examiner.ID,
		ScheduledAt: past.UTC(),
		Duration:    150,
		Status:      "pending",
	}

	require.NoError(t, s.CreateAppointment(app))
	assert.NotZero(t, app.ID)
}

func TestService_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	client := &Client{Name: "Status Client", Email: "status@test.com"}
	require.NoError(t, s.CreateClient(client))
	examiner := seedBookableExaminer(t, db)
	subject := seedSubject(t, db)

	app := &Appointment{
		ClientID:    client.ID,
		SubjectID:   subject.ID,
		ExaminerID:  examiner.ID,
		ScheduledAt: bookableTime(),
		Duration:    90,
		Status:      "pending",
	}
	require.NoError(t, s.CreateAppointment(app))

	// Update status
	idStr := fmt.Sprintf("%d", app.ID)
	err := s.UpdateStatus(idStr, "confirmed")
	assert.NoError(t, err)

	var updatedApp Appointment
	err = db.First(&updatedApp, app.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, "confirmed", updatedApp.Status)
}

func TestService_CreateAppointmentConvertsUSDFeeToAED(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS organization_settings (
			id INTEGER PRIMARY KEY,
			currency TEXT,
			usd_aed_rate REAL,
			usd_gbp_rate REAL,
			usd_eur_rate REAL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO organization_settings (id, currency, usd_aed_rate, usd_gbp_rate, usd_eur_rate)
		VALUES (1, 'AED', 3.6725, 0.7850, 0.9250)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS exam_types (
			id INTEGER PRIMARY KEY,
			name TEXT,
			price REAL,
			active INTEGER
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO exam_types (id, name, price, active) VALUES (1, 'Pre-employment Screening', 450, 1)
	`).Error)

	s := &Service{db: db}
	client := &Client{Name: "AED Client", Email: "aed@test.com"}
	require.NoError(t, s.CreateClient(client))
	examiner := seedBookableExaminer(t, db)
	subject := seedSubject(t, db)

	app := &Appointment{
		ClientID:    client.ID,
		SubjectID:   subject.ID,
		ExaminerID:  examiner.ID,
		ScheduledAt: bookableTime(),
		Duration:    150,
		ExamFee:     450,
		Status:      "pending",
		Notes:       "Pre-employment Screening\n\nTest booking",
	}
	require.NoError(t, s.CreateAppointment(app))

	assert.Equal(t, 1652.63, app.ExamFee)

	var quote Quotation
	require.NoError(t, db.Where("appointment_id = ?", app.ID).First(&quote).Error)
	assert.Equal(t, "AED", quote.Currency)
	assert.Equal(t, 1652.63, quote.Amount)
}

func TestService_BackfillRepairsSplitUSDFeeAndAEDCollected(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS organization_settings (
			id INTEGER PRIMARY KEY,
			currency TEXT,
			usd_aed_rate REAL,
			usd_gbp_rate REAL,
			usd_eur_rate REAL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO organization_settings (id, currency, usd_aed_rate, usd_gbp_rate, usd_eur_rate)
		VALUES (1, 'AED', 3.6725, 0.7850, 0.9250)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS exam_types (
			id INTEGER PRIMARY KEY,
			name TEXT,
			price REAL,
			active INTEGER
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO exam_types (id, name, price, active) VALUES (1, 'Civil Litigation Support', 550, 1)
	`).Error)

	s := &Service{db: db}
	client := &Client{Name: "EVA", Email: "eva@test.com"}
	require.NoError(t, s.CreateClient(client))
	examiner := seedBookableExaminer(t, db)
	subject := seedSubject(t, db)

	// Simulates corrupted production row after repeated currency conversion.
	appt := Appointment{
		ClientID:        client.ID,
		SubjectID:       subject.ID,
		ExaminerID:      examiner.ID,
		ScheduledAt:     bookableTime(),
		Duration:        150,
		ExamFee:         100048.60,
		FeeCurrency:     "USD",
		CollectedAmount: 100048.60,
		Status:          "completed",
		PaymentStatus:   "Paid",
		Notes:           "Civil Litigation Support\nLegacy import",
	}
	require.NoError(t, db.Create(&appt).Error)

	_, summary, err := s.buildBillingLedger(strconv.Itoa(int(client.ID)))
	require.NoError(t, err)
	// $550 USD = ~AED 2,020 at 3.6725
	assert.InDelta(t, 2020.0, summary.TotalBilled, 1.0)
	assert.InDelta(t, 2020.0, summary.TotalPaid, 1.0)
	assert.InDelta(t, 0, summary.BalanceDue, 0.02)

	var repaired Appointment
	require.NoError(t, db.First(&repaired, appt.ID).Error)
	assert.Equal(t, "AED", repaired.FeeCurrency)
	assert.InDelta(t, 2020.0, repaired.ExamFee, 1.0)
	assert.InDelta(t, 2020.0, repaired.CollectedAmount, 1.0)
}
