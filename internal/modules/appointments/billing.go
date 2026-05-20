package appointments

import (
	"strconv"
	"strings"
)

type examTypePriceRow struct {
	Name  string  `gorm:"column:name"`
	Price float64 `gorm:"column:price"`
}

func (s *Service) loadExamTypePriceIndex() map[string]float64 {
	var rows []examTypePriceRow
	_ = s.db.Table("exam_types").Where("active = ?", true).Select("name", "price").Find(&rows).Error
	index := make(map[string]float64, len(rows))
	for _, row := range rows {
		key := strings.ToLower(strings.TrimSpace(row.Name))
		if key != "" {
			index[key] = row.Price
		}
	}
	return index
}

func resolveFeeForAppointment(appt Appointment, typePrices map[string]float64) float64 {
	if appt.ExamFee > 0 {
		return appt.ExamFee
	}
	title := strings.ToLower(appointmentTitleFromNotes(appt.Notes))
	if price, ok := typePrices[title]; ok {
		return price
	}
	return 0
}

func (s *Service) backfillAppointmentBilling(appt *Appointment, typePrices map[string]float64) {
	if strings.EqualFold(strings.TrimSpace(appt.Status), "cancelled") {
		return
	}
	fee := resolveFeeForAppointment(*appt, typePrices)
	if fee <= 0 {
		return
	}
	if appt.ExamFee != fee {
		_ = s.db.Model(appt).Update("exam_fee", fee).Error
		appt.ExamFee = fee
	}
	_ = s.ensureInvoiceForAppointment(appt)
}

func appointmentTitleFromNotes(notes string) string {
	title := strings.TrimSpace(notes)
	if title == "" {
		return "Polygraph session"
	}
	if idx := strings.Index(title, "\n"); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	return title
}

func paymentStatusToQuoteStatus(paymentStatus string, collected, total float64) string {
	switch strings.ToLower(strings.TrimSpace(paymentStatus)) {
	case "paid":
		return "Completed"
	case "partial":
		return "Partial"
	default:
		if total > 0 && collected >= total {
			return "Completed"
		}
		if collected > 0 {
			return "Partial"
		}
		return "Pending"
	}
}

func quoteStatusToPaymentStatus(quoteStatus string, collected, total float64) string {
	switch strings.ToLower(strings.TrimSpace(quoteStatus)) {
	case "completed", "paid", "accepted":
		return "Paid"
	case "partial":
		return "Partial"
	default:
		if total > 0 && collected >= total {
			return "Paid"
		}
		if collected > 0 {
			return "Partial"
		}
		return "Unpaid"
	}
}

func (s *Service) ensureInvoiceForAppointment(appt *Appointment) error {
	if appt == nil || appt.ID == 0 {
		return nil
	}

	var existing int64
	if err := s.db.Model(&Quotation{}).Where("appointment_id = ?", appt.ID).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return s.syncQuotationFromAppointment(appt.ID)
	}

	if appt.ExamFee <= 0 && appt.CollectedAmount <= 0 {
		return nil
	}

	appointmentID := appt.ID
	quote := &Quotation{
		ClientID:        appt.ClientID,
		AppointmentID:   &appointmentID,
		Title:           appointmentTitleFromNotes(appt.Notes),
		Amount:          appt.ExamFee,
		CollectedAmount: appt.CollectedAmount,
		Status:          paymentStatusToQuoteStatus(appt.PaymentStatus, appt.CollectedAmount, appt.ExamFee),
	}
	if err := s.CreateQuotation(quote); err != nil {
		return err
	}
	return nil
}

func (s *Service) syncQuotationFromAppointment(appointmentID uint) error {
	var appt Appointment
	if err := s.db.First(&appt, appointmentID).Error; err != nil {
		return err
	}

	updates := map[string]interface{}{
		"amount":           appt.ExamFee,
		"collected_amount": appt.CollectedAmount,
		"status":           paymentStatusToQuoteStatus(appt.PaymentStatus, appt.CollectedAmount, appt.ExamFee),
	}
	return s.db.Model(&Quotation{}).Where("appointment_id = ?", appointmentID).Updates(updates).Error
}

func (s *Service) syncAppointmentFromQuotation(quotationID uint) error {
	var quote Quotation
	if err := s.db.First(&quote, quotationID).Error; err != nil {
		return err
	}
	if quote.AppointmentID == nil {
		return nil
	}

	updates := map[string]interface{}{
		"exam_fee":         quote.Amount,
		"collected_amount": quote.CollectedAmount,
		"payment_status":   quoteStatusToPaymentStatus(quote.Status, quote.CollectedAmount, quote.Amount),
	}
	return s.db.Model(&Appointment{}).Where("id = ?", *quote.AppointmentID).Updates(updates).Error
}

func (s *Service) buildBillingLedger(clientID string) ([]AccountLedgerEntry, AccountSummary, error) {
	trimmedClientID := strings.TrimSpace(clientID)

	var appointments []Appointment
	apptQuery := s.db.Preload("Client").Order("scheduled_at DESC")
	if trimmedClientID != "" {
		apptQuery = apptQuery.Where("client_id = ?", trimmedClientID)
	}
	if err := apptQuery.Find(&appointments).Error; err != nil {
		return nil, AccountSummary{}, err
	}

	typePrices := s.loadExamTypePriceIndex()
	for i := range appointments {
		s.backfillAppointmentBilling(&appointments[i], typePrices)
	}

	var quotations []Quotation
	quoteQuery := s.db.Preload("Client").Order("created_at DESC")
	if trimmedClientID != "" {
		quoteQuery = quoteQuery.Where("client_id = ?", trimmedClientID)
	}
	if err := quoteQuery.Find(&quotations).Error; err != nil {
		return nil, AccountSummary{}, err
	}

	apptByID := make(map[uint]Appointment, len(appointments))
	for _, appt := range appointments {
		apptByID[appt.ID] = appt
	}

	entries := make([]AccountLedgerEntry, 0, len(appointments)+len(quotations))
	var summary AccountSummary
	seenAppointments := make(map[uint]bool)
	seenQuotations := make(map[uint]bool)

	addEntry := func(entry AccountLedgerEntry) {
		entries = append(entries, entry)
		summary.TotalBilled += entry.TotalAmount
		summary.TotalPaid += entry.PaidAmount
	}

	for _, quote := range quotations {
		if quote.AppointmentID == nil {
			continue
		}
		appt, ok := apptByID[*quote.AppointmentID]
		if !ok {
			var loaded Appointment
			if err := s.db.Preload("Client").First(&loaded, *quote.AppointmentID).Error; err != nil {
				continue
			}
			appt = loaded
			apptByID[appt.ID] = appt
		}

		seenAppointments[appt.ID] = true
		seenQuotations[quote.ID] = true

		totalDue := resolveFeeForAppointment(appt, typePrices)
		balance := totalDue - appt.CollectedAmount
		if balance < 0 {
			balance = 0
		}
		code := strings.TrimSpace(quote.Code)
		if code == "" {
			code = "INV-" + strconv.Itoa(9000+int(quote.ID))
		}

		clientName := ""
		clientEmail := ""
		if quote.Client.ID != 0 {
			clientName = quote.Client.Name
			clientEmail = quote.Client.Email
		}
		if clientName == "" && appt.Client.ID != 0 {
			clientName = appt.Client.Name
			clientEmail = appt.Client.Email
		}

		qid := quote.ID
		aid := appt.ID
		addEntry(AccountLedgerEntry{
			ID:            quote.ID,
			Source:        "booking",
			Code:          code,
			ReferenceID:   appt.ID,
			AppointmentID: &aid,
			QuotationID:   &qid,
			ClientID:      appt.ClientID,
			ClientName:    clientName,
			ClientEmail:   clientEmail,
			Title:         appointmentTitleFromNotes(appt.Notes),
			Date:          appt.ScheduledAt,
			TotalAmount:   totalDue,
			PaidAmount:    appt.CollectedAmount,
			BalanceDue:    balance,
			Status:        appt.PaymentStatus,
			PaymentMode:   appt.PaymentMode,
		})
	}

	for _, appt := range appointments {
		if seenAppointments[appt.ID] {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(appt.Status), "cancelled") {
			continue
		}

		totalDue := resolveFeeForAppointment(appt, typePrices)
		if totalDue <= 0 && appt.CollectedAmount <= 0 && strings.TrimSpace(appt.Notes) == "" {
			continue
		}

		balance := totalDue - appt.CollectedAmount
		if balance < 0 {
			balance = 0
		}
		aid := appt.ID
		addEntry(AccountLedgerEntry{
			ID:            appt.ID,
			Source:        "session",
			Code:          formatAppointmentCode(appt.ID),
			ReferenceID:   appt.ID,
			AppointmentID: &aid,
			ClientID:      appt.ClientID,
			ClientName:    appt.Client.Name,
			ClientEmail:   appt.Client.Email,
			Title:         appointmentTitleFromNotes(appt.Notes),
			Date:          appt.ScheduledAt,
			TotalAmount:   totalDue,
			PaidAmount:    appt.CollectedAmount,
			BalanceDue:    balance,
			Status:        appt.PaymentStatus,
			PaymentMode:   appt.PaymentMode,
		})
	}

	for _, quote := range quotations {
		if seenQuotations[quote.ID] {
			continue
		}

		balance := quote.Amount - quote.CollectedAmount
		if balance < 0 {
			balance = 0
		}
		code := strings.TrimSpace(quote.Code)
		if code == "" {
			code = "INV-" + strconv.Itoa(9000+int(quote.ID))
		}

		qid := quote.ID
		addEntry(AccountLedgerEntry{
			ID:          quote.ID,
			Source:      "quote",
			Code:        code,
			ReferenceID: quote.ID,
			QuotationID: &qid,
			ClientID:    quote.ClientID,
			ClientName:  quote.Client.Name,
			ClientEmail: quote.Client.Email,
			Title:       quote.Title,
			Date:        quote.CreatedAt,
			TotalAmount: quote.Amount,
			PaidAmount:  quote.CollectedAmount,
			BalanceDue:  balance,
			Status:      quote.Status,
		})
	}

	summary.BalanceDue = summary.TotalBilled - summary.TotalPaid
	if summary.BalanceDue < 0 {
		summary.BalanceDue = 0
	}

	return entries, summary, nil
}
