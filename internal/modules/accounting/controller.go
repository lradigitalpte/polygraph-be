package accounting

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func parseOptionalDateRange(c *gin.Context) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	if raw := c.Query("from"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, nil, err
		}
		start := startOfUTCDay(t)
		from = &start
	}
	if raw := c.Query("to"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, nil, err
		}
		end := endOfUTCDay(t)
		to = &end
	}
	return from, to, nil
}

func (ctrl *Controller) ListExpenses(c *gin.Context) {
	from, to, err := parseOptionalDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from or to date (use YYYY-MM-DD)"})
		return
	}

	items, err := ctrl.service.ListExpenses(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (ctrl *Controller) GetExpense(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expense ID"})
		return
	}

	item, err := ctrl.service.GetExpense(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Expense not found"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (ctrl *Controller) CreateExpense(c *gin.Context) {
	var input CreateExpenseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var createdBy *uint
	if uid, ok := c.Get("user_id"); ok {
		if id, ok := uid.(uint); ok {
			createdBy = &id
		}
	}

	item, err := ctrl.service.CreateExpense(input, createdBy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (ctrl *Controller) UpdateExpense(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expense ID"})
		return
	}

	var input UpdateExpenseInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := ctrl.service.UpdateExpense(id, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (ctrl *Controller) DeleteExpense(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expense ID"})
		return
	}

	if err := ctrl.service.DeleteExpense(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Expense deleted"})
}

func (ctrl *Controller) GetVatReturn(c *gin.Context) {
	fromRaw := c.Query("from")
	toRaw := c.Query("to")
	if fromRaw == "" || toRaw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query params are required (YYYY-MM-DD)"})
		return
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from date"})
		return
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid to date"})
		return
	}

	report, err := ctrl.service.BuildVatReturn(from, to)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (ctrl *Controller) GetSalesReport(c *gin.Context) {
	fromRaw := c.Query("from")
	toRaw := c.Query("to")
	if fromRaw == "" || toRaw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to query params are required (YYYY-MM-DD)"})
		return
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from date"})
		return
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid to date"})
		return
	}

	report, err := ctrl.service.BuildSalesReport(from, to)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func parseIDParam(c *gin.Context) (uint, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
