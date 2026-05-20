package leads

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Controller exposes HTTP handlers for the leads module.
type Controller struct {
	service *Service
}

// NewController creates a Controller backed by the given Service.
func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

// Create godoc
// @Summary      Record a new lead intake
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        lead  body      CreateLeadInput  true  "Lead intake data"
// @Success      201   {object}  Lead
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /leads [post]
func (ctrl *Controller) Create(c *gin.Context) {
	var input CreateLeadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lead, err := ctrl.service.Create(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create lead"})
		return
	}

	c.JSON(http.StatusCreated, lead)
}

// GetAll godoc
// @Summary      List all leads
// @Tags         leads
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   Lead
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /leads [get]
func (ctrl *Controller) GetAll(c *gin.Context) {
	leads, err := ctrl.service.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch leads"})
		return
	}

	c.JSON(http.StatusOK, leads)
}

// GetByID godoc
// @Summary      Get a single lead
// @Tags         leads
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Lead ID"
// @Success      200  {object}  Lead
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /leads/{id} [get]
func (ctrl *Controller) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid lead ID"})
		return
	}

	lead, err := ctrl.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Lead not found"})
		return
	}

	c.JSON(http.StatusOK, lead)
}

// Update godoc
// @Summary      Update lead status or notes
// @Tags         leads
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int               true  "Lead ID"
// @Param        lead  body      UpdateLeadInput   true  "Fields to update"
// @Success      200   {object}  Lead
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /leads/{id} [patch]
func (ctrl *Controller) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid lead ID"})
		return
	}

	var input UpdateLeadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lead, err := ctrl.service.Update(uint(id), &input)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Lead not found or update failed"})
		return
	}

	c.JSON(http.StatusOK, lead)
}

// Delete godoc
// @Summary      Soft-delete a lead
// @Tags         leads
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Lead ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /leads/{id} [delete]
func (ctrl *Controller) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid lead ID"})
		return
	}

	if err := ctrl.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete lead"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lead deleted"})
}
