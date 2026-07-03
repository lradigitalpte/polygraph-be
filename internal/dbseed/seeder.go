package dbseed

import (
	"my-app/internal/modules/exams"
	"my-app/internal/modules/rbac"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SeedDatabase populates the database with initial permissions and roles
func SeedDatabase(db *gorm.DB, logger *zap.Logger) {
	// 1. Define Permissions
	perms := []rbac.Permission{
		// User Management
		{Name: "user:view", Description: "Can view user list", Group: "Users"},
		{Name: "user:create", Description: "Can create new users", Group: "Users"},
		{Name: "user:edit", Description: "Can edit existing users", Group: "Users"},
		{Name: "user:delete", Description: "Can delete users", Group: "Users"},

		// Role Management
		{Name: "role:manage", Description: "Can create and manage roles/permissions", Group: "Roles"},

		// Subject Management
		{Name: "subject:view", Description: "Can view examinees", Group: "Subjects"},
		{Name: "subject:create", Description: "Can register new examinees", Group: "Subjects"},
		{Name: "subject:edit", Description: "Can edit examinee profiles", Group: "Subjects"},

		// Client Management
		{Name: "client:view", Description: "Can view client details (read-only, no billing)", Group: "Clients"},
		{Name: "client:manage", Description: "Can manage agencies/clients", Group: "Clients"},

		// Appointment Management
		{Name: "appointment:manage", Description: "Can manage calendar/bookings", Group: "Appointments"},
		{Name: "appointment:view", Description: "Can view bookings", Group: "Appointments"},
		{Name: "appointment:create", Description: "Can create bookings", Group: "Appointments"},

		// Exam Management
		{Name: "exam:view", Description: "Can view exam sessions", Group: "Exams"},
		{Name: "exam:create", Description: "Can schedule/create exams", Group: "Exams"},
		{Name: "exam:conduct", Description: "Can conduct/record exams", Group: "Exams"},
		{Name: "exam:report", Description: "Can generate/sign forensic reports", Group: "Exams"},
		{Name: "exam:report:view_locked", Description: "Can view locked final forensic reports", Group: "Exams"},
		{Name: "exam:report:override", Description: "Can override locked reports for controlled revision", Group: "Exams"},
		{Name: "examtype:view", Description: "Can view exam types", Group: "Exams"},
		{Name: "examtype:create", Description: "Can create exam types", Group: "Exams"},
		{Name: "examtype:edit", Description: "Can edit exam types", Group: "Exams"},
		{Name: "examtype:delete", Description: "Can delete exam types", Group: "Exams"},

		// Availability Management
		{Name: "availability:view", Description: "Can view examiner availability blocks", Group: "Availability"},
		{Name: "availability:manage", Description: "Can manage examiner availability blocks", Group: "Availability"},
		{Name: "availability:check", Description: "Can check examiner availability during booking", Group: "Availability"},

		// Document Management
		{Name: "document:manage", Description: "Can manage forensic documents/charts", Group: "Exams"},

		// Lead Management
		{Name: "lead:view", Description: "Can view leads", Group: "Leads"},
		{Name: "lead:create", Description: "Can create new leads", Group: "Leads"},
		{Name: "lead:update", Description: "Can update lead status", Group: "Leads"},
		{Name: "lead:delete", Description: "Can delete leads", Group: "Leads"},

		// Audit Logs
		{Name: "audit:view", Description: "Can view system audit logs", Group: "Security"},

		// Payments / Billing
		{Name: "payment:view", Description: "Can view payments and financial billing", Group: "Payments"},
		{Name: "payment:manage", Description: "Can manage billing â€” collect, edit, and delete invoices", Group: "Payments"},

		// Reminders
		{Name: "reminder:view", Description: "Can view and send reminders", Group: "Reminders"},
	}

	for _, p := range perms {
		db.FirstOrCreate(&p, rbac.Permission{Name: p.Name})
	}

	SeedExamTypes(db)

	// 2. Define Roles
	//
	// NOTE: default role permissions are seeded ONLY when a role has no permissions
	// yet (first creation). After that, admins manage them via the Roles UI and we
	// must NOT clobber their changes on every boot. Admin is the exception â€” it
	// always gets every permission (including newly added ones).

	// ADMIN: always everything.
	var adminRole rbac.Role
	db.FirstOrCreate(&adminRole, rbac.Role{Name: "Admin"})
	var allPerms []rbac.Permission
	db.Find(&allPerms)
	db.Model(&adminRole).Association("Permissions").Replace(allPerms)

	// EXAMINER: Subjects and Exams (defaults only on first creation).
	var examinerRole rbac.Role
	db.FirstOrCreate(&examinerRole, rbac.Role{Name: "Examiner"})
	if db.Model(&examinerRole).Association("Permissions").Count() == 0 {
		var examinerPerms []rbac.Permission
		db.Where("\"group\" IN ? OR name IN ?", []string{"Subjects", "Exams"}, []string{"availability:view", "availability:manage", "appointment:view", "appointment:create", "availability:check", "client:view"}).Find(&examinerPerms)
		db.Model(&examinerRole).Association("Permissions").Replace(examinerPerms)
	}

	// USER: default role for auto-provisioned users (defaults only on first creation).
	var userRole rbac.Role
	db.FirstOrCreate(&userRole, rbac.Role{Name: "User"})
	if db.Model(&userRole).Association("Permissions").Count() == 0 {
		var userPerms []rbac.Permission
		db.Where("\"group\" IN ? OR name IN ?", []string{"Leads", "Clients", "Users"}, []string{"audit:view", "examtype:view", "appointment:view", "appointment:create", "availability:check", "subject:view", "subject:create", "payment:view", "payment:manage", "reminder:view"}).Find(&userPerms)
		db.Model(&userRole).Association("Permissions").Replace(userPerms)
	}

	logger.Info("Database seeding completed")
}

// SeedExamTypes restores the default exam type catalog.
func SeedExamTypes(db *gorm.DB) {
	defaultExamTypes := []exams.ExamType{
		{Name: "Pre-employment Screening", Description: "Baseline screening for employment or onboarding.", Category: "Screening", Duration: 150, Price: 450, Active: true},
		{Name: "Specific Issue Investigation", Description: "Focused investigation into a reported incident or allegation.", Category: "Investigation", Duration: 150, Price: 600, Active: true},
		{Name: "Periodic Maintenance", Description: "Recurring trust and compliance maintenance exam.", Category: "Maintenance", Duration: 120, Price: 350, Active: true},
		{Name: "Criminal Defense Exam", Description: "Defense-oriented forensic examination for criminal matters.", Category: "Legal", Duration: 150, Price: 700, Active: true},
		{Name: "Civil Litigation Support", Description: "Support examination for civil disputes and case preparation.", Category: "Legal", Duration: 150, Price: 550, Active: true},
		{Name: "Government/Security Clearance", Description: "Security and clearance review examination.", Category: "Security", Duration: 180, Price: 800, Active: true},
	}
	for _, examType := range defaultExamTypes {
		db.FirstOrCreate(&examType, exams.ExamType{Name: examType.Name})
	}
}


