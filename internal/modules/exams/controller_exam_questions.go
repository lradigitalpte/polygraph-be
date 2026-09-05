package exams

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (ctrl *Controller) ListExamQuestions(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam id"})
		return
	}
	questions, err := ctrl.service.ListExamQuestions(uint(examID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load exam questions"})
		return
	}
	c.JSON(http.StatusOK, questions)
}

func (ctrl *Controller) CreateExamQuestion(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam id"})
		return
	}
	var body struct {
		Text     string `json:"text"`
		Category string `json:"category"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	question, err := ctrl.service.CreateExamQuestion(uint(examID), body.Text, body.Category)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, question)
}

func (ctrl *Controller) UpdateExamQuestion(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam id"})
		return
	}
	questionID, err := strconv.ParseUint(c.Param("qid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid question id"})
		return
	}
	var input ExamQuestionUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	question, err := ctrl.service.UpdateExamQuestion(uint(examID), uint(questionID), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, question)
}

func (ctrl *Controller) DeleteExamQuestion(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam id"})
		return
	}
	questionID, err := strconv.ParseUint(c.Param("qid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid question id"})
		return
	}
	if err := ctrl.service.DeleteExamQuestion(uint(examID), uint(questionID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (ctrl *Controller) AddExamQuestionsFromTemplates(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam id"})
		return
	}
	var body struct {
		TemplateIDs []uint `json:"template_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	questions, err := ctrl.service.AddExamQuestionsFromTemplates(uint(examID), body.TemplateIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, questions)
}
