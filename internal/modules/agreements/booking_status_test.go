package agreements

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"my-app/internal/modules/appointments"
	"my-app/internal/modules/subjects"
)

func seedBooking(t *testing.T, s *Service, clientID uint) uint {
	t.Helper()
	subj := subjects.Subject{FirstName: "Jane", LastName: "Doe", ClientID: &clientID}
	require.NoError(t, s.db.Create(&subj).Error)
	appt := appointments.Appointment{ClientID: clientID, SubjectID: subj.ID, ScheduledAt: time.Now().Add(48 * time.Hour), Status: "confirmed"}
	require.NoError(t, s.db.Create(&appt).Error)
	return appt.ID
}

func TestBookingStatuses(t *testing.T) {
	s, _ := setupService(t)
	templates := seedTemplates(t, s)
	client := seedClient(t, s, "Individual")
	corporate := seedClient(t, s, "Corporate")

	notSent := seedBooking(t, s, client.ID)
	awaiting := seedBooking(t, s, client.ID)
	signed := seedBooking(t, s, client.ID)
	declined := seedBooking(t, s, client.ID)
	expired := seedBooking(t, s, client.ID)
	corp := seedBooking(t, s, corporate.ID)

	send := func(apptID uint) *SendResult {
		res, err := s.SendRequest(client.ID, SendInput{AppointmentID: &apptID, TemplateIDs: []uint{templates[0].ID}}, "staff@lab.test")
		require.NoError(t, err)
		return res
	}
	send(awaiting)

	// Signed wins even if an older request for the same booking was declined.
	old := send(signed)
	_, err := s.Decline(old.Request.Token, "")
	require.NoError(t, err)
	res := send(signed)
	view, err := s.GetPublicView(res.Request.Token)
	require.NoError(t, err)
	_, err = s.Sign(res.Request.Token, SignInput{SignedName: "Jane Doe", SignatureDataURL: testSignature(), AcceptedItemIDs: itemIDs(view.Items)}, "", "")
	require.NoError(t, err)

	d := send(declined)
	_, err = s.Decline(d.Request.Token, "no")
	require.NoError(t, err)

	e := send(expired)
	require.NoError(t, s.db.Model(&AgreementRequest{}).Where("id = ?", e.Request.ID).
		Update("expires_at", time.Now().Add(-time.Minute)).Error)

	statuses, err := s.BookingStatuses([]uint{notSent, awaiting, signed, declined, expired, corp, 99999})
	require.NoError(t, err)
	assert.Equal(t, BookingNotSent, statuses[notSent].State)
	assert.Equal(t, BookingAwaiting, statuses[awaiting].State)
	assert.Equal(t, BookingSigned, statuses[signed].State)
	assert.Equal(t, res.Request.ID, statuses[signed].RequestID)
	assert.NotNil(t, statuses[signed].SignedAt)
	assert.Equal(t, BookingDeclined, statuses[declined].State)
	assert.Equal(t, BookingNotSent, statuses[expired].State, "a lapsed link counts as not sent")
	assert.NotContains(t, statuses, corp, "corporate bookings are skipped")
	assert.NotContains(t, statuses, uint(99999))
}

func TestParseAppointmentIDs(t *testing.T) {
	ids, err := ParseAppointmentIDs(" 3,1, 3 ,,2")
	require.NoError(t, err)
	assert.Equal(t, []uint{1, 2, 3}, ids)
	_, err = ParseAppointmentIDs("1,abc")
	assert.Error(t, err)
	_, err = ParseAppointmentIDs("0")
	assert.Error(t, err)
	ids, err = ParseAppointmentIDs("")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
