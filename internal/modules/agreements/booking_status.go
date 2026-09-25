package agreements

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Booking agreement states, as shown on calendar and booking views. These only ever
// produce warnings; nothing in the booking or payment flow is blocked by them.
const (
	BookingSigned   = "signed"
	BookingAwaiting = "awaiting"
	BookingDeclined = "declined"
	BookingNotSent  = "not_sent"
)

const maxBookingStatusIDs = 500

type BookingAgreementStatus struct {
	State     string     `json:"state"`
	RequestID uint       `json:"request_id,omitempty"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	SignedAt  *time.Time `json:"signed_at,omitempty"`
}

// ParseAppointmentIDs parses a comma-separated id list ("1,2,3").
func ParseAppointmentIDs(raw string) ([]uint, error) {
	var ids []uint
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil || id == 0 {
			return nil, errors.New("appointment_ids must be a comma-separated list of ids")
		}
		ids = append(ids, uint(id))
	}
	ids = uniqueIDs(ids)
	if len(ids) > maxBookingStatusIDs {
		return nil, errors.New("too many appointment ids")
	}
	return ids, nil
}

// statePriority ranks requests for one booking: a signed request outranks one awaiting
// signature, which outranks a decline. Expired and voided requests don't count.
var statePriority = map[string]int{StatusSigned: 3, StatusSent: 2, StatusViewed: 2, StatusDeclined: 1}

// BookingStatuses returns the agreement state for each booking of an individual client.
// Bookings under corporate or law-firm accounts are omitted, since they don't sign
// agreements online.
func (s *Service) BookingStatuses(appointmentIDs []uint) (map[uint]BookingAgreementStatus, error) {
	out := map[uint]BookingAgreementStatus{}
	if len(appointmentIDs) == 0 {
		return out, nil
	}

	var bookings []struct {
		ID         uint
		ClientType string
	}
	err := s.db.Table("appointments").
		Select("appointments.id, clients.client_type").
		Joins("JOIN clients ON clients.id = appointments.client_id").
		Where("appointments.id IN ? AND appointments.deleted_at IS NULL", appointmentIDs).
		Scan(&bookings).Error
	if err != nil {
		return nil, err
	}
	for _, b := range bookings {
		if !isOrganizationClientType(b.ClientType) {
			out[b.ID] = BookingAgreementStatus{State: BookingNotSent}
		}
	}

	// Newest first, so among requests of equal priority the latest wins.
	countable := []string{StatusSigned, StatusSent, StatusViewed, StatusDeclined}
	var requests []AgreementRequest
	err = s.db.Select("id", "appointment_id", "status", "sent_at", "signed_at", "expires_at").
		Where("appointment_id IN ? AND status IN ?", appointmentIDs, countable).
		Order("sent_at DESC").
		Find(&requests).Error
	if err != nil {
		return nil, err
	}

	best := map[uint]AgreementRequest{}
	now := time.Now().UTC()
	for _, r := range requests {
		if r.AppointmentID == nil {
			continue
		}
		if _, tracked := out[*r.AppointmentID]; !tracked {
			continue
		}
		if r.isOpen() && now.After(r.ExpiresAt) {
			continue // lapsed link: treated as not sent
		}
		if cur, ok := best[*r.AppointmentID]; !ok || statePriority[r.Status] > statePriority[cur.Status] {
			best[*r.AppointmentID] = r
		}
	}
	for apptID, r := range best {
		state := BookingDeclined
		switch r.Status {
		case StatusSigned:
			state = BookingSigned
		case StatusSent, StatusViewed:
			state = BookingAwaiting
		}
		sentAt := r.SentAt
		out[apptID] = BookingAgreementStatus{State: state, RequestID: r.ID, SentAt: &sentAt, SignedAt: r.SignedAt}
	}
	return out, nil
}
