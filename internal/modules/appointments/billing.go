package appointments

import (
	"math"
	"strconv"
	"strings"

	"my-app/internal/money"
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

func (s *Service) loadMoneyRates() money.Rates {
	return money.LoadRates(s.db)
}

func orgCurrencyCode(rates money.Rates) string {
	code := strings.ToUpper(strings.TrimSpace(rates.Currency))
	if code == "" {
		return "USD"
	}
	return code
}

func catalogUSDForAppointment(appt Appointment, typePrices map[string]float64) (float64, bool) {
	title := strings.ToLower(appointmentTitleFromNotes(appt.Notes))
	price, ok := typePrices[title]
	return price, ok
}

// inferLegacyFeeCurrency guesses whether exam_fee is still in USD for rows created before fee_currency existed.
func inferLegacyFeeCurrency(appt Appointment, catalogUSD float64, rates money.Rates) string {
	org := orgCurrencyCode(rates)
	if org == "USD" {
		return "USD"
	}
	if appt.ExamFee <= 0 {
		return org
	}
	if catalogUSD > 0 && money.ApproxEqual(appt.ExamFee, catalogUSD) {
		return "USD"
	}
	if catalogUSD > 0 {
		expectedOrg := money.Round2(money.FromUSD(catalogUSD, rates))
		if money.ApproxEqual(appt.ExamFee, expectedOrg) {
			return org
		}
	}
	// Fee left in USD while collected was converted to org currency (common after historical import).
	if appt.CollectedAmount > appt.ExamFee*1.5 {
		convertedFee := money.FromUSD(appt.ExamFee, rates)
		if money.ApproxEqual(appt.CollectedAmount, convertedFee) || math.Abs(appt.CollectedAmount-convertedFee) < 1 {
			return "USD"
		}
	}
	// Both amounts still at legacy USD scale.
	if appt.CollectedAmount > 0 &&
		money.ApproxEqual(appt.CollectedAmount, appt.ExamFee) &&
		catalogUSD > 0 &&
		appt.ExamFee <= catalogUSD*1.25 {
		return "USD"
	}
	if catalogUSD > 0 {
		expectedOrg := money.Round2(money.FromUSD(catalogUSD, rates))
		if appt.ExamFee >= expectedOrg*0.98 {
			return org
		}
	}
	return "USD"
}

func appointmentFeeCurrency(appt Appointment, typePrices map[string]float64, rates money.Rates) string {
	if cur := strings.ToUpper(strings.TrimSpace(appt.FeeCurrency)); cur != "" {
		return cur
	}
	catalogUSD, _ := catalogUSDForAppointment(appt, typePrices)
	return inferLegacyFeeCurrency(appt, catalogUSD, rates)
}

func amountInOrgCurrency(amount float64, currency string, rates money.Rates) float64 {
	org := orgCurrencyCode(rates)
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if cur == "" {
		cur = "USD"
	}
	if org == cur {
		return money.Round2(amount)
	}
	return money.Round2(money.Convert(amount, cur, org, rates))
}

// resolveUSDFeeForAppointment returns the catalog fee in USD (exam_types.price is USD).
func resolveUSDFeeForAppointment(appt Appointment, typePrices map[string]float64) float64 {
	if appt.ExamFee > 0 {
		return appt.ExamFee
	}
	title := strings.ToLower(appointmentTitleFromNotes(appt.Notes))
	if price, ok := typePrices[title]; ok {
		return price
	}
	return 0
}

// resolveFeeInOrgCurrency returns the appointment fee in the organization's billing currency.
func (s *Service) resolveFeeInOrgCurrency(appt Appointment, typePrices map[string]float64, rates money.Rates) float64 {
	if appt.ExamFee <= 0 {
		catalogUSD, ok := catalogUSDForAppointment(appt, typePrices)
		if !ok || catalogUSD <= 0 {
			return 0
		}
		return money.Round2(money.FromUSD(catalogUSD, rates))
	}
	feeCurrency := appointmentFeeCurrency(appt, typePrices, rates)
	return amountInOrgCurrency(appt.ExamFee, feeCurrency, rates)
}

func (s *Service) normalizeCollectedInOrgCurrency(appt Appointment, orgFee float64, feeCurrency string, rates money.Rates) float64 {
	if appt.CollectedAmount <= 0 {
		return 0
	}
	org := orgCurrencyCode(rates)
	cur := strings.ToUpper(strings.TrimSpace(feeCurrency))
	if cur == "" {
		cur = "USD"
	}

	// Collected was recorded in the same currency as the stored fee.
	if cur != org {
		if org != "USD" && appt.ExamFee > 0 {
			convertedFee := money.FromUSD(appt.ExamFee, rates)
			if money.ApproxEqual(appt.CollectedAmount, convertedFee) || math.Abs(appt.CollectedAmount-convertedFee) < 1 {
				return money.Round2(appt.CollectedAmount)
			}
		}
		return amountInOrgCurrency(appt.CollectedAmount, cur, rates)
	}

	// Legacy rows may still hold collected in USD while fee is already org currency.
	if org != "USD" && appt.ExamFee > 0 && !money.ApproxEqual(appt.ExamFee, orgFee) {
		return money.Round2(orgFee * (appt.CollectedAmount / appt.ExamFee))
	}
	if org != "USD" && appt.ExamFee <= 0 {
		return amountInOrgCurrency(appt.CollectedAmount, "USD", rates)
	}
	return money.Round2(appt.CollectedAmount)
}

func (s *Service) normalizeAppointmentMoney(appt *Appointment, typePrices map[string]float64, rates money.Rates) {
	if appt == nil {
		return
	}
	org := orgCurrencyCode(rates)
	feeCurrency := appointmentFeeCurrency(*appt, typePrices, rates)
	orgFee := amountInOrgCurrency(appt.ExamFee, feeCurrency, rates)
	orgCollected := s.normalizeCollectedInOrgCurrency(*appt, orgFee, feeCurrency, rates)

	if orgCollected > 0 && strings.EqualFold(strings.TrimSpace(appt.PaymentStatus), "Paid") {
		if math.Abs(orgFee-orgCollected) < 1 || orgCollected > orgFee {
			orgFee = orgCollected
		}
	}

	if money.ApproxEqual(appt.ExamFee, orgFee) &&
		money.ApproxEqual(appt.CollectedAmount, orgCollected) &&
		strings.EqualFold(strings.TrimSpace(appt.FeeCurrency), org) {
		return
	}

	updates := map[string]interface{}{
		"exam_fee":     orgFee,
		"fee_currency": org,
	}
	if !money.ApproxEqual(appt.CollectedAmount, orgCollected) {
		updates["collected_amount"] = orgCollected
	}
	_ = s.db.Model(&Appointment{}).Where("id = ?", appt.ID).Updates(updates).Error
	appt.ExamFee = orgFee
	appt.FeeCurrency = org
	appt.CollectedAmount = orgCollected
}

func (s *Service) backfillAppointmentBilling(appt *Appointment, typePrices map[string]float64) {
	if strings.EqualFold(strings.TrimSpace(appt.Status), "cancelled") {
		return
	}
	rates := s.loadMoneyRates()
	s.normalizeAppointmentMoney(appt, typePrices, rates)
	if appt.ExamFee <= 0 && appt.CollectedAmount <= 0 {
		return
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
	if idx := strings.Index(title, " | "); idx > 0 {
		title = strings.TrimSpace(title[:idx])
	}
	return truncate(title, 255)
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

	billingCurrency := orgCurrencyCode(s.loadMoneyRates())

	appointmentID := appt.ID
	quote := &Quotation{
		ClientID:        appt.ClientID,
		AppointmentID:   &appointmentID,
		Title:           appointmentTitleFromNotes(appt.Notes),
		Amount:          appt.ExamFee,
		CollectedAmount: appt.CollectedAmount,
		Status:          paymentStatusToQuoteStatus(appt.PaymentStatus, appt.CollectedAmount, appt.ExamFee),
		Currency:        billingCurrency,
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

	typePrices := s.loadExamTypePriceIndex()
	rates := s.loadMoneyRates()
	s.normalizeAppointmentMoney(&appt, typePrices, rates)

	billingCurrency := orgCurrencyCode(rates)

	updates := map[string]interface{}{
		"amount":           appt.ExamFee,
		"collected_amount": appt.CollectedAmount,
		"status":           paymentStatusToQuoteStatus(appt.PaymentStatus, appt.CollectedAmount, appt.ExamFee),
		"currency":         billingCurrency,
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
		"fee_currency":     strings.ToUpper(strings.TrimSpace(quote.Currency)),
		"payment_status":   quoteStatusToPaymentStatus(quote.Status, quote.CollectedAmount, quote.Amount),
	}
	return s.db.Model(&Appointment{}).Where("id = ?", *quote.AppointmentID).Updates(updates).Error
}

func (s *Service) buildBillingLedger(clientID string) ([]AccountLedgerEntry, AccountSummary, error) {
	trimmedClientID := strings.TrimSpace(clientID)
	rates := s.loadMoneyRates()
	defaultCurrency := orgCurrencyCode(rates)

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

		totalDue := appt.ExamFee
		if totalDue <= 0 {
			totalDue = s.resolveFeeInOrgCurrency(appt, typePrices, rates)
		}
		if totalDue <= 0 {
			totalDue = amountInOrgCurrency(quote.Amount, quote.Currency, rates)
		}
		paid := appt.CollectedAmount
		if paid <= 0 {
			paid = amountInOrgCurrency(quote.CollectedAmount, quote.Currency, rates)
		}
		balance := totalDue - paid
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
			PaidAmount:    paid,
			BalanceDue:    balance,
			Status:        appt.PaymentStatus,
			PaymentMode:   appt.PaymentMode,
			Currency:      defaultCurrency,
		})
	}

	for _, appt := range appointments {
		if seenAppointments[appt.ID] {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(appt.Status), "cancelled") {
			continue
		}

		totalDue := appt.ExamFee
		if totalDue <= 0 {
			totalDue = s.resolveFeeInOrgCurrency(appt, typePrices, rates)
		}
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
			Currency:      defaultCurrency,
		})
	}

	for _, quote := range quotations {
		if seenQuotations[quote.ID] {
			continue
		}

		total := amountInOrgCurrency(quote.Amount, quote.Currency, rates)
		paid := amountInOrgCurrency(quote.CollectedAmount, quote.Currency, rates)

		balance := total - paid
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
			TotalAmount: total,
			PaidAmount:  paid,
			BalanceDue:  balance,
			Status:      quote.Status,
			Currency:    defaultCurrency,
		})
	}

	summary.BalanceDue = summary.TotalBilled - summary.TotalPaid
	if summary.BalanceDue < 0 {
		summary.BalanceDue = 0
	}

	return entries, summary, nil
}

// NormalizeNewAppointmentMoney converts incoming catalog USD fees to org billing currency.
func (s *Service) NormalizeNewAppointmentMoney(app *Appointment) {
	if app == nil {
		return
	}
	rates := s.loadMoneyRates()
	if strings.TrimSpace(app.FeeCurrency) == "" {
		app.FeeCurrency = "USD"
	}
	typePrices := s.loadExamTypePriceIndex()
	s.normalizeAppointmentMoney(app, typePrices, rates)
}
