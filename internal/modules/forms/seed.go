package forms

import (
	"encoding/json"

	"gorm.io/gorm"
)

func SeedTemplates(db *gorm.DB) {
	templates := []struct {
		Slug        string
		Name        string
		Category    string
		Description string
		Audience    string
		Schema      FormSchema
	}{
		{
			Slug:        "consent-examination",
			Name:        "Consent to Examination",
			Category:    "consent",
			Description: "Informed consent before polygraph testing.",
			Audience:    "all",
			Schema: FormSchema{Fields: []FormField{
				{Key: "full_name", Label: "Full legal name", Type: "text", Required: true},
				{Key: "date_of_birth", Label: "Date of birth", Type: "text", Required: true},
				{Key: "id_number", Label: "ID / passport number", Type: "text", Required: false},
				{Key: "consent_voluntary", Label: "I voluntarily consent to the examination", Type: "checkbox", Required: true},
				{Key: "consent_questions", Label: "I understand I may decline to answer specific questions", Type: "checkbox", Required: true},
				{Key: "signature_ack", Label: "I confirm the information above is accurate", Type: "checkbox", Required: true},
			}},
		},
		{
			Slug:        "privacy-notice",
			Name:        "Privacy & Data Notice",
			Category:    "privacy",
			Description: "Acknowledgement of privacy practices and data handling.",
			Audience:    "all",
			Schema: FormSchema{Fields: []FormField{
				{Key: "full_name", Label: "Full name", Type: "text", Required: true},
				{Key: "email", Label: "Email address", Type: "email", Required: true},
				{Key: "privacy_read", Label: "I have read the privacy notice", Type: "checkbox", Required: true},
				{Key: "data_processing", Label: "I consent to processing of my data for this examination", Type: "checkbox", Required: true},
			}},
		},
		{
			Slug:        "legal-authorization",
			Name:        "Legal Authorization",
			Category:    "legal",
			Description: "Authorization from legal counsel or referring agency where applicable.",
			Audience:    "all",
			Schema: FormSchema{Fields: []FormField{
				{Key: "client_organization", Label: "Organization / law firm", Type: "text", Required: false},
				{Key: "examinee_name", Label: "Examinee full name", Type: "text", Required: true},
				{Key: "matter_reference", Label: "Matter / case reference", Type: "text", Required: false},
				{Key: "authorizing_party", Label: "Authorizing party name", Type: "text", Required: true},
				{Key: "authorization_confirm", Label: "I am authorized to request this examination", Type: "checkbox", Required: true},
			}},
		},
		{
			Slug:        "client-intake",
			Name:        "Client Intake Questionnaire",
			Category:    "intake",
			Description: "Background and reason for examination.",
			Audience:    "all",
			Schema: FormSchema{Fields: []FormField{
				{Key: "full_name", Label: "Full name", Type: "text", Required: true},
				{Key: "phone", Label: "Phone", Type: "text", Required: false},
				{Key: "email", Label: "Email", Type: "email", Required: true},
				{Key: "reason_for_exam", Label: "Reason for examination", Type: "textarea", Required: true},
				{Key: "medications", Label: "Current medications (if any)", Type: "textarea", Required: false},
				{Key: "additional_notes", Label: "Additional notes", Type: "textarea", Required: false},
			}},
		},
		{
			Slug:        "examinee-prep",
			Name:        "Examinee Preparation Checklist",
			Category:    "intake",
			Description: "Pre-session instructions acknowledgement for the person being tested.",
			Audience:    "examinee",
			Schema: FormSchema{Fields: []FormField{
				{Key: "examinee_name", Label: "Your full name", Type: "text", Required: true},
				{Key: "prep_rest", Label: "I had adequate rest before the session", Type: "checkbox", Required: true},
				{Key: "prep_caffeine", Label: "I avoided caffeine for 4+ hours before testing", Type: "checkbox", Required: true},
				{Key: "prep_id", Label: "I will bring valid photo ID to the session", Type: "checkbox", Required: true},
			}},
		},
	}

	for _, t := range templates {
		raw, _ := json.Marshal(t.Schema)
		tpl := FormTemplate{
			Slug:        t.Slug,
			Name:        t.Name,
			Category:    t.Category,
			Description: t.Description,
			Audience:    t.Audience,
			SchemaJSON:  string(raw),
			Version:     1,
			Active:      true,
		}
		db.Where(FormTemplate{Slug: t.Slug}).Assign(tpl).FirstOrCreate(&tpl)
	}
}
