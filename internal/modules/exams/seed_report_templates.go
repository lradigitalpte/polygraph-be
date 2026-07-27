package exams

import (
	"encoding/json"

	"gorm.io/gorm"
)

func templateContent(data StructuredReport) string {
	b, _ := json.Marshal(data)
	return string(b)
}

func reportQuestions(items [][3]string) []StructuredReportQuestion {
	out := make([]StructuredReportQuestion, 0, len(items))
	for _, item := range items {
		out = append(out, StructuredReportQuestion{
			Text:       item[0],
			Answer:     item[1],
			Evaluation: item[2],
		})
	}
	return out
}

func SeedReportTemplates(db *gorm.DB) {
	makeTemplate := func(slug, name, category, description string, isDefault bool, data StructuredReport) ReportTemplate {
		return ReportTemplate{
			Slug:        slug,
			Name:        name,
			Category:    category,
			Description: description,
			ContentJSON: templateContent(data),
			IsDefault:   isDefault,
			Active:      true,
		}
	}

	templates := []ReportTemplate{
		makeTemplate(
			"standard-walk-in",
			"Standard / Walk-in",
			"generic",
			"Neutral pre-employment template for walk-in clients and general corporate use.",
			true,
			StructuredReport{
				Purpose:                  "A screening polygraph test was administered as part of a pre-employment assessment for {{client_name}}.",
				Instrument:               "Lafayette Instruments Polygraph",
				PreTestPhaseText:         "On {{exam_date}} at about {{exam_start_time}} hrs (Dubai Time), I commenced to administer a polygraph examination to the above subject.",
				PreExamQuestionCountText: "4 relevant and 3 comparison questions",
				PreTestNotes:             "The examinee signed a formal consent form confirming their voluntary agreement to undergo the polygraph examination.",
				ExamPhaseText:            "During the examination phase, the relevant and comparison questions were administered to subject with a set of 4 relevant questions. {{pronoun_possessive}} verbal responses to the relevant questions were as indicated:",
				Questions: reportQuestions([][3]string{
					{"Have you ever provided false information on an employment application?", "No", "No Reaction"},
					{"Have you ever stolen money or property from an employer?", "No", "No Reaction"},
					{"Have you ever used company resources for personal gain without authorization?", "No", "No Reaction"},
					{"Is your purpose in applying for this position to intentionally harm or undermine the organization?", "No", "No Reaction"},
				}),
				LimeToneNotes:            "The examination was conducted with a Lafayette Instruments Polygraph, recording the blood pressure, pulse rate, galvanic skin response and breathing pattern of the subject.\nFour polygrams, including 1 acquaintance and 3 official tests were recorded, and the process ended at about {{exam_end_time}} hrs (Dubai Time).",
				OpinionPhaseText:         "Based on the diagnostic evaluations and analysis of the polygrams, I am of the opinion that the examination conducted on {{subject_name}} concluded as {{verdict_label}}.",
				PostTestNotes:            "{{cooperation_sentence}}",
				Section4FollowUp:         "Nil",
				IdentityDocumentType:     "passport",
				ExamStartTime:            "14:00",
				ExamEndTime:              "15:45",
				CooperationMode:          "cooperated",
				IdentityVerificationText: "{{identity_sentence}}",
			},
		),
		makeTemplate(
			"eva-pre-employment",
			"Eva – Pre-employment",
			"eva",
			"Default Eva corporate pre-employment screening questions.",
			false,
			StructuredReport{
				Purpose:                  "A screening polygraph test was administered as part of a pre-employment test for {{client_name}}.",
				Instrument:               "Lafayette Instruments Polygraph",
				PreTestPhaseText:         "On {{exam_date}} at about {{exam_start_time}} hrs (Dubai Time), I commenced to administer a polygraph examination to the above subject.",
				PreExamQuestionCountText: "4 relevant and 3 comparison questions",
				PreTestNotes:             "The structured pre-examination interview was conducted in English during which {{pre_exam_question_count_text}} were developed. The questions were reduced into writing and reviewed with subject word for word.\n\nThe examinee signed a formal consent form confirming their voluntary agreement to undergo the polygraph examination.",
				ExamPhaseText:            "During the examination phase, the relevant and comparison questions were administered to subject with a set of 4 relevant questions. {{pronoun_possessive}} verbal responses to the relevant questions were as indicated:",
				Questions: reportQuestions([][3]string{
					{"Have you ever shared confidential company information with an unauthorized person?", "No", "No Reaction"},
					{"Have you stolen money, leads or any property from a company you worked for?", "No", "No Reaction"},
					{"Have you ever used company resources like leads or tools for your own personal gain or for someone else?", "No", "No Reaction"},
					{"Is your purpose in applying for this position to intentionally damage or undermine the company?", "No", "No Reaction"},
				}),
				LimeToneNotes:            "The examination was conducted with a Lafayette Instruments Polygraph, recording the blood pressure, pulse rate, galvanic skin response and breathing pattern of the subject.\nFour polygrams, including 1 acquaintance and 3 official tests were recorded, and the process ended at about {{exam_end_time}} hrs (Dubai Time).",
				OpinionPhaseText:         "Based on the diagnostic evaluations and analysis of the polygrams, I am of the opinion that the examination conducted on {{subject_name}} concluded as {{verdict_label}}.",
				PostTestNotes:            "{{cooperation_sentence}}",
				Section4FollowUp:         "Nil",
				IdentityDocumentType:     "passport",
				ExamStartTime:            "14:00",
				ExamEndTime:              "15:45",
				CooperationMode:          "cooperated",
				IdentityVerificationText: "{{identity_sentence}}",
			},
		),
		makeTemplate(
			"eva-manager",
			"Eva – Manager",
			"eva",
			"Manager-specific investigation questions for Eva corporate clients.",
			false,
			StructuredReport{
				Purpose:                  "A screening polygraph test was administered as part of a pre-employment test for {{client_name}}.",
				Instrument:               "Limestone Technologies Computerised Polygraph",
				PreTestPhaseText:         "On {{exam_date}} at about {{exam_start_time}} hrs (Dubai Time), I commenced to administer a polygraph examination to the above subject.",
				PreExamQuestionCountText: "4 relevant and 3 comparison questions",
				PreTestNotes:             "The structured pre-examination interview was conducted in English during which {{pre_exam_question_count_text}} were developed. The questions were reduced into writing and reviewed with subject word for word.",
				ExamPhaseText:            "During the examination phase, the relevant and comparison questions were administered to subject with a set of 4 relevant questions. {{pronoun_possessive}} verbal responses to the relevant questions were as indicated (Responses highlighted in red indicate deception, while those highlighted in green indicate truthfulness):",
				ResponseLegendText:       "Responses highlighted in red indicate deception, while those highlighted in green indicate truthfulness.",
				Questions: reportQuestions([][3]string{
					{"Did you make deals with the company or Afaf without the knowledge of Sharon and peter?", "No", "No Reaction"},
					{"Have you shared information to Police or CID about the activity of the company?", "No", "No Reaction"},
					{"Did you raise the prices for your own gain?", "No", "No Reaction"},
					{"Did you keep data of the company in order to gain money or security without the knowledge of your colleagues?", "No", "No Reaction"},
				}),
				LimeToneNotes:            "The examination was conducted with a Limestone Technologies Computerised Polygraph, recording the blood pressure, pulse rate, galvanic skin response and breathing pattern of the subject.\nFour polygrams, including 1 acquaintance and 3 official tests were recorded, and the process ended at about {{exam_end_time}} hrs (Dubai Time).",
				OpinionPhaseText:         "Based on the diagnostic evaluations and analysis of the polygrams, I am in the opinion that the examination on {{subject_name}} concluded as {{verdict_label}}.",
				PostTestNotes:            "{{cooperation_sentence}}",
				Section4FollowUp:         "Nil",
				IdentityDocumentType:     "passport",
				ExamStartTime:            "13:30",
				ExamEndTime:              "15:10",
				CooperationMode:          "cooperated",
				IdentityVerificationText: "{{identity_sentence}}",
			},
		),
		makeTemplate(
			"eva-reception-office-boy",
			"Eva – Reception / Office Boy",
			"eva",
			"Reception and office boy role questions for Eva corporate clients.",
			false,
			StructuredReport{
				Purpose:                  "A screening polygraph test was administered as part of a pre-employment test for {{client_name}}.",
				Instrument:               "Limestone Technologies Computerised Polygraph",
				PreTestPhaseText:         "On {{exam_date}} at about {{exam_start_time}} hrs (Dubai Time), I commenced to administer a polygraph examination to the above subject.",
				PreExamQuestionCountText: "4 relevant and 3 comparison questions",
				PreTestNotes:             "The structured pre-examination interview was conducted in English during which {{pre_exam_question_count_text}} were developed. The questions were reduced into writing and reviewed with subject word for word.",
				ExamPhaseText:            "During the examination phase, the relevant and comparison questions were administered to subject with a set of 4 relevant questions. {{pronoun_possessive}} verbal responses to the relevant questions were as indicated:",
				Questions: reportQuestions([][3]string{
					{"Have you ever caused any damage or harm to a company you previously worked for?", "No", "No Reaction"},
					{"Have you ever cooperated with law enforcement authorities for the purpose of sharing confidential information related to a company you worked for or were applying to?", "No", "No Reaction"},
					{"Do you have any hidden or personal motives for wanting to work for this company?", "No", "No Reaction"},
					{"Have you ever shared confidential company information with third parties?", "No", "No Reaction"},
				}),
				LimeToneNotes:            "The examination was conducted with a Limestone Technologies Computerised Polygraph, recording the blood pressure, pulse rate, galvanic skin response and breathing pattern of the subject.\nFour polygrams, including 1 acquaintance and 3 official tests were recorded, and the process ended at about {{exam_end_time}} hrs (Dubai Time).",
				OpinionPhaseText:         "Based on the diagnostic evaluations and analysis of the polygrams, I am in the opinion that the examination on {{subject_name}} as {{verdict_label}}.",
				PostTestNotes:            "{{cooperation_sentence}}",
				Section4FollowUp:         "Nil",
				IdentityDocumentType:     "passport",
				ExamStartTime:            "14:00",
				ExamEndTime:              "15:45",
				CooperationMode:          "cooperated",
				IdentityVerificationText: "{{identity_sentence}}",
			},
		),
	}

	for _, tpl := range templates {
		var existing ReportTemplate
		if err := db.Where("slug = ?", tpl.Slug).First(&existing).Error; err == nil {
			continue
		}
		db.Create(&tpl)
	}

	var defaultCount int64
	db.Model(&ReportTemplate{}).Where("is_default = ? AND active = ?", true, true).Count(&defaultCount)
	if defaultCount == 0 {
		db.Model(&ReportTemplate{}).Where("slug = ?", "standard-walk-in").Update("is_default", true)
	}
}
