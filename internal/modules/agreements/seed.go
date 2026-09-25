package agreements

import "gorm.io/gorm"

// SeedTemplates adds starter payment, reschedule and cancellation agreements the first
// time the table is empty. They are placeholders for the lab to edit in Settings; once
// any agreement exists (or all were deleted on purpose after that) nothing is re-seeded.
func SeedTemplates(db *gorm.DB) {
	var count int64
	db.Unscoped().Model(&AgreementTemplate{}).Count(&count)
	if count > 0 {
		return
	}
	starters := []AgreementTemplate{
		{
			Title:     "Payment Agreement",
			Kind:      KindPayment,
			SortOrder: 1,
			BodyHTML: `<p>By signing this agreement you confirm the following payment terms for your booked examination.</p>
<ol><li><p>The full examination fee shown on your booking is payable before the session begins.</p></li>
<li><p>Accepted payment methods are bank transfer, card and cash.</p></li>
<li><p>The session may be postponed if payment has not been received by the scheduled start time.</p></li></ol>
<p><em>Edit these terms in Settings → Agreements before sending to clients.</em></p>`,
		},
		{
			Title:     "Reschedule Policy",
			Kind:      KindReschedule,
			SortOrder: 2,
			BodyHTML: `<p>You may reschedule your examination under the following conditions.</p>
<ol><li><p>Requests made at least 48 hours before the session can be rescheduled free of charge.</p></li>
<li><p>Requests made less than 48 hours before the session may incur a rescheduling fee.</p></li>
<li><p>A booking can be rescheduled a maximum of two times.</p></li></ol>
<p><em>Edit these terms in Settings → Agreements before sending to clients.</em></p>`,
		},
		{
			Title:     "Cancellation Policy",
			Kind:      KindCancellation,
			SortOrder: 3,
			BodyHTML: `<p>If you cancel your examination, the following applies.</p>
<ol><li><p>Cancellations made at least 48 hours before the session receive a full refund.</p></li>
<li><p>Cancellations made less than 48 hours before the session may be charged a cancellation fee.</p></li>
<li><p>Failing to attend without notice is treated as a late cancellation.</p></li></ol>
<p><em>Edit these terms in Settings → Agreements before sending to clients.</em></p>`,
		},
	}
	for _, t := range starters {
		t.Version = 1
		t.Active = true
		t.BodyHTML = SanitizeTermsHTML(t.BodyHTML)
		db.Create(&t)
	}
}
