package agreements

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/modules/appointments"
)

// ErrNotSigned is returned when a PDF is requested for agreements that are not signed.
var ErrNotSigned = errors.New("a PDF copy is only available once the agreements are signed")

func signedPDFFilename(id uint) string {
	return fmt.Sprintf("Signed-agreement-%s.pdf", agreementReference(id))
}

// ensurePDF returns the stored signed copy, generating and storing it on first use.
// The first stored copy wins, so every later download is the identical file.
func (s *Service) ensurePDF(requestID uint) ([]byte, string, error) {
	var req AgreementRequest
	err := s.db.Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC") }).
		First(&req, requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrRequestNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if req.Status != StatusSigned {
		return nil, "", ErrNotSigned
	}
	filename := signedPDFFilename(req.ID)
	if len(req.PDFData) > 0 {
		return req.PDFData, filename, nil
	}

	pdfBytes, err := generateAgreementPDF(pdfInput{
		Request:      req,
		Organization: s.publicOrganization(),
		Booking:      s.bookingFor(req.AppointmentID),
	})
	if err != nil {
		log.Printf("agreements: generate PDF for request %d: %v", req.ID, err)
		return nil, "", errors.New("the PDF copy could not be generated")
	}
	sum := sha256.Sum256(pdfBytes)
	hash := hex.EncodeToString(sum[:])

	res := s.db.Model(&AgreementRequest{}).
		Where("id = ? AND (pdf_sha256 IS NULL OR pdf_sha256 = '')", req.ID).
		Updates(map[string]interface{}{"pdf_data": pdfBytes, "pdf_sha256": hash})
	if res.Error != nil {
		return nil, "", res.Error
	}
	if res.RowsAffected == 0 {
		// Another request stored a copy first; serve that one.
		var stored AgreementRequest
		if err := s.db.Select("pdf_data").First(&stored, req.ID).Error; err != nil {
			return nil, "", err
		}
		return stored.PDFData, filename, nil
	}

	s.saveToDocumentVault(&req, pdfBytes, hash)
	return pdfBytes, filename, nil
}

// saveToDocumentVault files the signed copy under the client's documents. Best effort:
// the agreement record itself remains the source of truth for downloads.
func (s *Service) saveToDocumentVault(req *AgreementRequest, pdfBytes []byte, hash string) {
	if s.storage == nil || req.ClientDocumentID != nil {
		return
	}
	key := fmt.Sprintf("clients/%d/agreements/%s", req.ClientID, signedPDFFilename(req.ID))
	url, err := s.storage.UploadFile(context.Background(), key, bytes.NewReader(pdfBytes), "application/pdf")
	if err != nil {
		log.Printf("agreements: upload PDF for request %d: %v", req.ID, err)
		return
	}
	titles := make([]string, 0, len(req.Items))
	for _, it := range req.Items {
		titles = append(titles, it.Title)
	}
	name := fmt.Sprintf("Signed agreement %s (%s)", agreementReference(req.ID), strings.Join(titles, ", "))
	if len(name) > 250 {
		name = name[:247] + "..."
	}
	doc := appointments.ClientDocument{
		ClientID: req.ClientID,
		Name:     name,
		Type:     "agreement",
		Source:   "online_agreement",
		URL:      url,
		Hash:     hash,
	}
	if err := s.db.Create(&doc).Error; err != nil {
		log.Printf("agreements: vault document for request %d: %v", req.ID, err)
		return
	}
	s.db.Model(&AgreementRequest{}).Where("id = ?", req.ID).Update("client_document_id", doc.ID)
}

// PublicPDF serves the signed copy to the client through their signing link.
func (s *Service) PublicPDF(token string) ([]byte, string, error) {
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, "", err
	}
	return s.ensurePDF(req.ID)
}

// StaffPDF serves the signed copy to staff from the Approvals tab.
func (s *Service) StaffPDF(id string) ([]byte, string, error) {
	req, err := s.loadRequest(id)
	if err != nil {
		return nil, "", err
	}
	return s.ensurePDF(req.ID)
}
