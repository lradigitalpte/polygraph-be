package exams

import (
	"encoding/json"
	"strings"
)

type ReportMergeContext struct {
	SubjectName        string
	ClientName         string
	ExamDate           string
	ReportDate         string
	ReferenceNo        string
	ExamStartTime      string
	ExamEndTime        string
	SubjectGender      string
	IdentityDocType    string
	CooperationMode    string
	VerdictLabel       string
	PreExamQuestionCnt string
}

func normalizeGender(gender string) string {
	return strings.ToLower(strings.TrimSpace(gender))
}

func pronounsForGender(gender string) (subject, possessive, object string) {
	switch normalizeGender(gender) {
	case "female", "f", "woman", "women":
		return "She", "Her", "her"
	case "male", "m", "man", "men":
		return "He", "His", "him"
	default:
		return "They", "Their", "them"
	}
}

func identitySentence(docType string) string {
	docType = strings.ToLower(strings.TrimSpace(docType))
	label := "Passport or Emirates ID"
	switch docType {
	case "passport":
		label = "Passport"
	case "emirates_id", "emirates-id", "emirates id":
		label = "Emirates ID"
	}
	return "The examiner verified the examinee's identity through an official " + label + " in accordance with standard procedures."
}

func cooperationSentence(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "counter_measures") {
		return "Examinee employed counter measures to cheat the test."
	}
	return "Examinee cooperated and the test administration was as per procedure."
}

func mergeTemplatePlaceholders(text string, ctx ReportMergeContext) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	_, possessive, _ := pronounsForGender(ctx.SubjectGender)
	pronounSubject, _, _ := pronounsForGender(ctx.SubjectGender)
	replacer := strings.NewReplacer(
		"{{subject_name}}", ctx.SubjectName,
		"{{client_name}}", ctx.ClientName,
		"{{exam_date}}", ctx.ExamDate,
		"{{report_date}}", ctx.ReportDate,
		"{{reference_no}}", ctx.ReferenceNo,
		"{{exam_start_time}}", ctx.ExamStartTime,
		"{{exam_end_time}}", ctx.ExamEndTime,
		"{{pronoun_subject}}", pronounSubject,
		"{{pronoun_possessive}}", possessive,
		"{{identity_sentence}}", identitySentence(ctx.IdentityDocType),
		"{{cooperation_sentence}}", cooperationSentence(ctx.CooperationMode),
		"{{verdict_label}}", ctx.VerdictLabel,
		"{{pre_exam_question_count_text}}", ctx.PreExamQuestionCnt,
	)
	return replacer.Replace(text)
}

func mergeStructuredReport(data StructuredReport, ctx ReportMergeContext) StructuredReport {
	merged := data
	merged.Purpose = mergeTemplatePlaceholders(data.Purpose, ctx)
	merged.PreTestNotes = mergeTemplatePlaceholders(data.PreTestNotes, ctx)
	merged.PreTestPhaseText = mergeTemplatePlaceholders(data.PreTestPhaseText, ctx)
	merged.ExamPhaseText = mergeTemplatePlaceholders(data.ExamPhaseText, ctx)
	merged.LimeToneNotes = mergeTemplatePlaceholders(data.LimeToneNotes, ctx)
	merged.OpinionPhaseText = mergeTemplatePlaceholders(data.OpinionPhaseText, ctx)
	merged.PostTestNotes = mergeTemplatePlaceholders(data.PostTestNotes, ctx)
	merged.Section4FollowUp = mergeTemplatePlaceholders(data.Section4FollowUp, ctx)
	merged.ResponseLegendText = mergeTemplatePlaceholders(data.ResponseLegendText, ctx)
	merged.IdentityVerificationText = mergeTemplatePlaceholders(data.IdentityVerificationText, ctx)
	if strings.TrimSpace(merged.IdentityVerificationText) == "" || merged.IdentityVerificationText == "{{identity_sentence}}" {
		merged.IdentityVerificationText = identitySentence(ctx.IdentityDocType)
	}
	merged.ReferenceNo = ctx.ReferenceNo
	merged.ExamDate = ctx.ExamDate
	merged.ReportDate = ctx.ReportDate
	if strings.TrimSpace(merged.ExamStartTime) == "" {
		merged.ExamStartTime = ctx.ExamStartTime
	}
	if strings.TrimSpace(merged.ExamEndTime) == "" {
		merged.ExamEndTime = ctx.ExamEndTime
	}
	if strings.TrimSpace(merged.IdentityDocumentType) == "" {
		merged.IdentityDocumentType = ctx.IdentityDocType
	}
	if strings.TrimSpace(merged.CooperationMode) == "" {
		merged.CooperationMode = ctx.CooperationMode
	}
	if strings.TrimSpace(merged.PreExamQuestionCountText) == "" {
		merged.PreExamQuestionCountText = ctx.PreExamQuestionCnt
	}
	return merged
}

func parseTemplateContent(raw string) (StructuredReport, error) {
	var data StructuredReport
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return StructuredReport{}, err
	}
	return data, nil
}
