package availability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

type blockInput struct {
	ExaminerID uint   `json:"examiner_id" binding:"required"`
	Date       string `json:"date" binding:"required"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	IsFullDay  bool   `json:"is_full_day"`
	Reason     string `json:"reason"`
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (ctrl *Controller) ListBlocks(c *gin.Context) {
	examinerID, _ := strconv.ParseUint(c.Query("examiner_id"), 10, 64)
	blocks, err := ctrl.service.ListBlocks(uint(examinerID), c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, blocks)
}

func (ctrl *Controller) GetExaminerDayAvailability(c *gin.Context) {
	examinerID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid examiner id"})
		return
	}

	date := c.Query("date")
	if date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required"})
		return
	}

	blocks, busy, isBlocked, listErr := ctrl.service.GetExaminerDaySchedule(uint(examinerID), date)
	if listErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": listErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"examiner_id":   uint(examinerID),
		"date":          date,
		"is_blocked":    isBlocked,
		"blocks":        blocks,
		"busy_periods":  busy,
	})
}

func (ctrl *Controller) CreateBlock(c *gin.Context) {
	var input blockInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	date, err := time.Parse("2006-01-02", input.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must use YYYY-MM-DD"})
		return
	}

	block := Block{
		ExaminerID: input.ExaminerID,
		Date:       date,
		StartTime:  strings.TrimSpace(input.StartTime),
		EndTime:    strings.TrimSpace(input.EndTime),
		IsFullDay:  input.IsFullDay,
		Reason:     strings.TrimSpace(input.Reason),
	}

	if err := ctrl.service.CreateBlock(&block); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, block)
}

func (ctrl *Controller) UpdateBlock(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid availability block id"})
		return
	}

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	block, updateErr := ctrl.service.UpdateBlock(uint(id), payload)
	if updateErr != nil {
		status := http.StatusBadRequest
		if strings.Contains(updateErr.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": updateErr.Error()})
		return
	}
	c.JSON(http.StatusOK, block)
}

func (ctrl *Controller) DeleteBlock(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid availability block id"})
		return
	}
	if err := ctrl.service.DeleteBlock(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete availability block"})
		return
	}
	c.Status(http.StatusNoContent)
}
