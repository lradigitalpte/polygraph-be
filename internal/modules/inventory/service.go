package inventory

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"my-app/internal/database"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

type CreateItemInput struct {
	Name           string     `json:"name"`
	SerialNumber   string     `json:"serial_number"`
	Category       string     `json:"category"`
	Status         string     `json:"status"`
	Quantity       int        `json:"quantity"`
	Location       string     `json:"location"`
	PurchaseDate   *time.Time `json:"purchase_date"`
	WarrantyExpiry *time.Time `json:"warranty_expiry"`
	CalibrationDue *time.Time `json:"calibration_due"`
	ExpirationDate *time.Time `json:"expiration_date"`
	Notes          string     `json:"notes"`
}

type UpdateItemInput struct {
	Name           string     `json:"name"`
	SerialNumber   string     `json:"serial_number"`
	Category       string     `json:"category"`
	Status         string     `json:"status"`
	Quantity       int        `json:"quantity"`
	Location       string     `json:"location"`
	PurchaseDate   *time.Time `json:"purchase_date"`
	WarrantyExpiry *time.Time `json:"warranty_expiry"`
	CalibrationDue *time.Time `json:"calibration_due"`
	ExpirationDate *time.Time `json:"expiration_date"`
	Notes          string     `json:"notes"`
}

func (s *Service) ListItems(search string, category string, status string) ([]InventoryItem, error) {
	var items []InventoryItem
	query := s.db.Model(&InventoryItem{})

	search = strings.TrimSpace(search)
	if search != "" {
		likePattern := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(name) LIKE ? OR LOWER(serial_number) LIKE ? OR LOWER(location) LIKE ?", likePattern, likePattern, likePattern)
	}

	category = strings.TrimSpace(category)
	if category != "" && category != "All" {
		query = query.Where("category = ?", category)
	}

	status = strings.TrimSpace(status)
	if status != "" && status != "All" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("name ASC").Find(&items).Error
	return items, err
}

func (s *Service) GetItem(id uint) (*InventoryItem, error) {
	var item InventoryItem
	err := s.db.First(&item, id).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) CreateItem(input CreateItemInput) (*InventoryItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("item name is required")
	}

	qty := input.Quantity
	if qty < 0 {
		qty = 1
	}

	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "Equipment"
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "Active"
	}

	item := InventoryItem{
		Name:           name,
		SerialNumber:   strings.TrimSpace(input.SerialNumber),
		Category:       category,
		Status:         status,
		Quantity:       qty,
		Location:       strings.TrimSpace(input.Location),
		PurchaseDate:   input.PurchaseDate,
		WarrantyExpiry: input.WarrantyExpiry,
		CalibrationDue: input.CalibrationDue,
		ExpirationDate: input.ExpirationDate,
		Notes:          strings.TrimSpace(input.Notes),
	}

	err := s.db.Create(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) UpdateItem(id uint, input UpdateItemInput) (*InventoryItem, error) {
	item, err := s.GetItem(id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("item name is required")
	}

	qty := input.Quantity
	if qty < 0 {
		qty = 1
	}

	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "Equipment"
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "Active"
	}

	updates := map[string]interface{}{
		"name":            name,
		"serial_number":   strings.TrimSpace(input.SerialNumber),
		"category":        category,
		"status":          status,
		"quantity":        qty,
		"location":        strings.TrimSpace(input.Location),
		"purchase_date":   input.PurchaseDate,
		"warranty_expiry": input.WarrantyExpiry,
		"calibration_due": input.CalibrationDue,
		"expiration_date": input.ExpirationDate,
		"notes":           strings.TrimSpace(input.Notes),
	}

	err = s.db.Model(item).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	return s.GetItem(id)
}

func (s *Service) DeleteItem(id uint) error {
	return s.db.Delete(&InventoryItem{}, id).Error
}
