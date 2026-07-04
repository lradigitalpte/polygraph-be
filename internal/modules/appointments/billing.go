package appointments

import (
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

func catalogOrgFee(appt Appointment, typePrices map[string]float64, rates money.Rates) (float64, bool) {
	catalogUSD, ok := catalogUSDForAppointment(appt, typePrices)
	if !ok || catalogUSD <= 0 {
		return 0, false
	}
	return money.Round2(money.FromUSD(catalogUSD, rates)), true
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
	if target, ok := catalogOrgFee(appt, typePrices, rates); ok {
		return target
	}
	if appt.ExamFee <= 0 {
		return 0
	}
	org := orgCurrencyCode(rates)
	cur := strings.ToUpper(strings.TrimSpace(appt.FeeCurrency))
	if cur == org || (cur == "" && org == "USD") {
		return money.Round2(appt.ExamFee)
	}
	if cur == "USD" || cur == "" {
		return amountInOrgCurrency(appt.ExamFee, "USD", rates)
	}
	return money.Round2(appt.ExamFee)
}

func (s *Service) normalizeCollectedForTarget(appt Appointment, targetFee float64, rates money.Rates) float64 {
	if appt.CollectedAmount <= 0 {
		return 0
	}
	org := orgCurrencyCode(rates)
	if targetFee <= 0 {
		cur := strings.ToUpper(strings.TrimSpace(appt.FeeCurrency))
		if cur == org {
			return money.Round2(appt.CollectedAmount)
		}
		return amountInOrgCurrency(appt.CollectedAmount, "USD", rates)
	}
	if strings.EqualFold(strings.TrimSpace(appt.PaymentStatus), "Paid") {
		return targetFee
	}
	collected := appt.CollectedAmount
	cur := strings.ToUpper(strings.TrimSpace(appt.FeeCurrency))
	if cur == "USD" || (cur == "" && collected <= targetFee*0.5) {
		collected = amountInOrgCurrency(collected, "USD", rates)
	}
	if collected > targetFee*1.05 {
		return targetFee
	}
	return money.Round2(collected)
}

// canonicalAppointmentAmounts returns org-currency fee and collected amounts.
// Exam type catalog (USD) is the source of truth when the appointment notes identify a type.
func (s *Service) canonicalAppointmentAmounts(appt Appointment, typePrices map[string]float64, rates money.Rates) (float64, float64) {
	org := orgCurrencyCode(rates)

	if target, ok := catalogOrgFee(appt, typePrices, rates); ok {
		collected := s.normalizeCollectedForTarget(appt, target, rates)
		return target, collected
	}

	fee := appt.ExamFee
	cur := strings.ToUpper(strings.TrimSpace(appt.FeeCurrency))
	if cur == org || (cur == "" && org == "USD") {
		fee = money.Round2(fee)
	} else {
		fee = amountInOrgCurrency(fee, "USD", rates)
	}
	collected := s.normalizeCollectedForTarget(appt, fee, rates)
	return fee, collected
}

func (s *Service) normalizeAppointmentMoney(appt *Appointment, typePrices map[string]float64, rates money.Rates) {
	if appt == nil {
		return
	}
	org := orgCurrencyCode(rates)
	orgFee, orgCollected := s.canonicalAppointmentAmounts(*appt, typePrices, rates)

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

	org := orgCurrencyCode(s.loadMoneyRates())
	feeCurrency := strings.ToUpper(strings.TrimSpace(quote.Currency))
	if feeCurrency == "" {
		feeCurrency = org
	}

	updates := map[string]interface{}{
		"exam_fee":         quote.Amount,
		"collected_amount": quote.CollectedAmount,
		"fee_currency":     feeCurrency,
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

		totalDue, paid := s.canonicalAppointmentAmounts(appt, typePrices, rates)
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

		totalDue, paid := s.canonicalAppointmentAmounts(appt, typePrices, rates)
		if totalDue <= 0 && paid <= 0 && strings.TrimSpace(appt.Notes) == "" {
			continue
		}

		balance := totalDue - paid
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
			PaidAmount:    paid,
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

		quoteCur := strings.ToUpper(strings.TrimSpace(quote.Currency))
		if quoteCur == "" {
			quoteCur = defaultCurrency
		}
		total := amountInOrgCurrency(quote.Amount, quoteCur, rates)
		paid := amountInOrgCurrency(quote.CollectedAmount, quoteCur, rates)

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
	if strings.TrimSpace(app.FeeCurrency) == "" {
		app.FeeCurrency = "USD"
	}
	typePrices := s.loadExamTypePriceIndex()
	rates := s.loadMoneyRates()
	s.normalizeAppointmentMoney(app, typePrices, rates)
}
