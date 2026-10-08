package accounting

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (ctrl *Controller) ListPaymentMethods(c *gin.Context) {
	items, err := ctrl.service.ListPaymentMethods()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (ctrl *Controller) CreatePaymentMethod(c *gin.Context) {
	var input PaymentMethodInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := ctrl.service.CreatePaymentMethod(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (ctrl *Controller) UpdatePaymentMethod(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var input PaymentMethodInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := ctrl.service.UpdatePaymentMethod(id, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (ctrl *Controller) DeletePaymentMethod(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	if err := ctrl.service.DeletePaymentMethod(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (ctrl *Controller) ListPurchaseItems(c *gin.Context) {
	search := c.Query("search")
	includeInactive := c.Query("include_inactive") == "true"
	items, err := ctrl.service.ListPurchaseItems(search, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (ctrl *Controller) CreatePurchaseItem(c *gin.Context) {
	var input PurchaseItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := ctrl.service.CreatePurchaseItem(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (ctrl *Controller) UpdatePurchaseItem(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	var input PurchaseItemInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := ctrl.service.UpdatePurchaseItem(id, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (ctrl *Controller) DeletePurchaseItem(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}
	if err := ctrl.service.DeletePurchaseItem(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (ctrl *Controller) UploadExpenseReceipt(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expense ID"})
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	item, err := ctrl.service.UploadExpenseReceipt(c.Request.Context(), id, header.Filename, file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (ctrl *Controller) GetExpenseReceipt(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expense ID"})
		return
	}
	url, name, err := ctrl.service.ExpenseReceiptDownloadURL(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "file_name": name})
}
