package specification

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Table-driven tests for comprehensive scenario coverage

func TestSpecificationTable(t *testing.T) {
	// Create test users with various properties
	users := []*TestUser{
		{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true},  // Valid all
		{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true},      // Under 18
		{ID: 3, Name: "Charlie", Age: 30, Email: "", IsActive: false},                // No email, inactive
		{ID: 4, Name: "Diana", Age: 22, Email: "invalid.email", IsActive: true},      // Invalid email
		{ID: 5, Name: "Eve", Age: 70, Email: "eve@example.com", IsActive: true},      // Senior
		{ID: 6, Name: "Frank", Age: 16, Email: "frank@test.com", IsActive: false},    // Minor, inactive
	}

	tests := []struct {
		name        string
		spec        Specification[TestUser]
		user        *TestUser
		shouldPass  bool
		description string
	}{
		// Active Specification Tests
		{"Active_Alice", &UserActiveSpec{}, users[0], true, "Active user should pass"},
		{"Active_Charlie", &UserActiveSpec{}, users[2], false, "Inactive user should fail"},
		{"Active_Frank", &UserActiveSpec{}, users[5], false, "Inactive minor should fail"},

		// Age Min Specification Tests  
		{"AgeMin18_Alice", &UserAgeMinSpec{MinAge: 18}, users[0], true, "Adult should pass age check"},
		{"AgeMin18_Bob", &UserAgeMinSpec{MinAge: 18}, users[1], false, "Minor should fail age check"},
		{"AgeMin18_Eve", &UserAgeMinSpec{MinAge: 18}, users[4], true, "Senior should pass age check"},
		{"AgeMin21_Alice", &UserAgeMinSpec{MinAge: 21}, users[0], true, "25-year-old should pass 21+ check"},
		{"AgeMin21_Bob", &UserAgeMinSpec{MinAge: 21}, users[1], false, "17-year-old should fail 21+ check"},

		// Age Max Specification Tests
		{"AgeMax65_Alice", &UserAgeMaxSpec{MaxAge: 65}, users[0], true, "Young adult should pass max age"},
		{"AgeMax65_Eve", &UserAgeMaxSpec{MaxAge: 65}, users[4], false, "Senior should fail max age"},
		{"AgeMax30_Bob", &UserAgeMaxSpec{MaxAge: 30}, users[1], true, "Teen should pass max 30"},

		// Email Specification Tests
		{"Email_Alice", &UserEmailValidSpec{}, users[0], true, "Valid email should pass"},
		{"Email_Charlie", &UserEmailValidSpec{}, users[2], false, "Empty email should fail"},
		{"Email_Diana", &UserEmailValidSpec{}, users[3], false, "Invalid email should fail"},
		{"Email_Eve", &UserEmailValidSpec{}, users[4], true, "Valid senior email should pass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.IsSatisfiedBy(tt.user)
			
			if tt.shouldPass {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}

func TestComplexSpecificationTable(t *testing.T) {
	users := createTestUsers()

	tests := []struct {
		name         string
		spec         Specification[TestUser]
		expectedPass []string // Names of users who should pass
		description  string
	}{
		{
			name: "ActiveAdults",
			spec: NewAndSpecification[TestUser](
				&UserActiveSpec{},
				&UserAgeMinSpec{MinAge: 18},
			),
			expectedPass: []string{"Alice", "Diana", "Eve", "Henry"}, // Active users 18+
			description:  "Should find active users who are adults",
		},
		{
			name: "YoungOrSenior",
			spec: NewOrSpecification[TestUser](
				&UserAgeMaxSpec{MaxAge: 20},  // Young
				&UserAgeMinSpec{MinAge: 65},  // Senior
			),
			expectedPass: []string{"Bob", "Frank"}, // Young users only (no seniors in test data)
			description:  "Should find users who are either young or senior",
		},
		{
			name: "NotActive",
			spec: NewNotSpecification[TestUser](&UserActiveSpec{}),
			expectedPass: []string{"Charlie", "Grace"}, // Inactive users
			description:  "Should find inactive users",
		},
		{
			name: "ActiveWithValidEmail",
			spec: NewAndSpecification[TestUser](
				&UserActiveSpec{},
				&UserEmailValidSpec{},
			),
			expectedPass: []string{"Alice", "Eve"}, // Active with valid email
			description:  "Should find active users with valid email",
		},
		{
			name: "ComplexBusiness",
			spec: NewOrSpecification[TestUser](
				// VIP: Active seniors
				NewAndSpecification[TestUser](
					&UserActiveSpec{},
					&UserAgeMinSpec{MinAge: 30},
				),
				// Special: Young with valid email
				NewAndSpecification[TestUser](
					&UserAgeMaxSpec{MaxAge: 25},
					&UserEmailValidSpec{},
				),
			),
			expectedPass: []string{"Alice", "Eve"}, // Alice (25, valid email) and Eve (35, active senior)
			description:  "Complex business rule: VIP seniors OR special young users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Filter(users, tt.spec)

			// Verify expected users are in result
			resultNames := make([]string, len(result))
			for i, user := range result {
				resultNames[i] = user.Name
			}

			assert.ElementsMatch(t, tt.expectedPass, resultNames, tt.description)

			// If some users were expected to pass, but not all, there should be errors
			if len(tt.expectedPass) > 0 && len(tt.expectedPass) < len(users) {
				assert.Error(t, err, "Should have errors for users who don't match criteria")
			}
		})
	}
}

// Parameterized AND specification tests
func TestAndSpecificationTable(t *testing.T) {
	user := &TestUser{ID: 1, Name: "Test", Age: 25, Email: "test@example.com", IsActive: true}

	tests := []struct {
		name       string
		specs      []Specification[TestUser]
		shouldPass bool
		reason     string
	}{
		{
			name:       "AllPass",
			specs:      []Specification[TestUser]{&AlwaysPassSpec[TestUser]{}, &AlwaysPassSpec[TestUser]{}},
			shouldPass: true,
			reason:     "All specs pass, AND should pass",
		},
		{
			name:       "OneFails",
			specs:      []Specification[TestUser]{&AlwaysPassSpec[TestUser]{}, &AlwaysFailSpec[TestUser]{}},
			shouldPass: false,
			reason:     "One spec fails, AND should fail",
		},
		{
			name:       "AllFail",
			specs:      []Specification[TestUser]{&AlwaysFailSpec[TestUser]{}, &AlwaysFailSpec[TestUser]{}},
			shouldPass: false,
			reason:     "All specs fail, AND should fail",
		},
		{
			name:       "Empty",
			specs:      []Specification[TestUser]{},
			shouldPass: true,
			reason:     "No specs to fail, AND should pass",
		},
		{
			name:       "Single",
			specs:      []Specification[TestUser]{&UserActiveSpec{}},
			shouldPass: true,
			reason:     "Single passing spec, AND should pass",
		},
		{
			name:       "RealWorld",
			specs:      []Specification[TestUser]{&UserActiveSpec{}, &UserAgeMinSpec{MinAge: 18}, &UserEmailValidSpec{}},
			shouldPass: true,
			reason:     "Real user meets all criteria",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			andSpec := NewAndSpecification[TestUser](tt.specs...)
			err := andSpec.IsSatisfiedBy(user)

			if tt.shouldPass {
				assert.NoError(t, err, tt.reason)
			} else {
				assert.Error(t, err, tt.reason)
			}
		})
	}
}

// Parameterized OR specification tests
func TestOrSpecificationTable(t *testing.T) {
	user := &TestUser{ID: 1, Name: "Test", Age: 25, Email: "test@example.com", IsActive: true}

	tests := []struct {
		name       string
		specs      []Specification[TestUser]
		shouldPass bool
		reason     string
	}{
		{
			name:       "AllPass",
			specs:      []Specification[TestUser]{&AlwaysPassSpec[TestUser]{}, &AlwaysPassSpec[TestUser]{}},
			shouldPass: true,
			reason:     "All specs pass, OR should pass",
		},
		{
			name:       "OnePasses",
			specs:      []Specification[TestUser]{&AlwaysPassSpec[TestUser]{}, &AlwaysFailSpec[TestUser]{}},
			shouldPass: true,
			reason:     "One spec passes, OR should pass",
		},
		{
			name:       "AllFail",
			specs:      []Specification[TestUser]{&AlwaysFailSpec[TestUser]{}, &AlwaysFailSpec[TestUser]{}},
			shouldPass: false,
			reason:     "All specs fail, OR should fail",
		},
		{
			name:       "Empty",
			specs:      []Specification[TestUser]{},
			shouldPass: true,
			reason:     "No specs to fail, OR should pass",
		},
		{
			name:       "LastPasses",
			specs:      []Specification[TestUser]{&AlwaysFailSpec[TestUser]{}, &AlwaysFailSpec[TestUser]{}, &UserActiveSpec{}},
			shouldPass: true,
			reason:     "Last spec passes, OR should pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orSpec := NewOrSpecification[TestUser](tt.specs...)
			err := orSpec.IsSatisfiedBy(user)

			if tt.shouldPass {
				assert.NoError(t, err, tt.reason)
			} else {
				assert.Error(t, err, tt.reason)
			}
		})
	}
}

// NOT specification behavior table
func TestNotSpecificationTable(t *testing.T) {
	tests := []struct {
		name       string
		innerSpec  Specification[TestUser]
		user       *TestUser
		shouldPass bool
		reason     string
	}{
		{
			name:       "NotActive_ActiveUser",
			innerSpec:  &UserActiveSpec{},
			user:       &TestUser{IsActive: true},
			shouldPass: false,
			reason:     "NOT(active) should fail for active user",
		},
		{
			name:       "NotActive_InactiveUser",
			innerSpec:  &UserActiveSpec{},
			user:       &TestUser{IsActive: false},
			shouldPass: true,
			reason:     "NOT(active) should pass for inactive user",
		},
		{
			name:       "NotAlwaysPass",
			innerSpec:  &AlwaysPassSpec[TestUser]{},
			user:       &TestUser{},
			shouldPass: false,
			reason:     "NOT(always pass) should fail",
		},
		{
			name:       "NotAlwaysFail",
			innerSpec:  &AlwaysFailSpec[TestUser]{},
			user:       &TestUser{},
			shouldPass: true,
			reason:     "NOT(always fail) should pass",
		},
		{
			name:       "NotAgeMin_YoungUser",
			innerSpec:  &UserAgeMinSpec{MinAge: 18},
			user:       &TestUser{Age: 16},
			shouldPass: true,
			reason:     "NOT(age ≥ 18) should pass for minor",
		},
		{
			name:       "NotAgeMin_AdultUser",
			innerSpec:  &UserAgeMinSpec{MinAge: 18},
			user:       &TestUser{Age: 25},
			shouldPass: false,
			reason:     "NOT(age ≥ 18) should fail for adult",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notSpec := NewNotSpecification[TestUser](tt.innerSpec)
			err := notSpec.IsSatisfiedBy(tt.user)

			if tt.shouldPass {
				assert.NoError(t, err, tt.reason)
			} else {
				assert.Error(t, err, tt.reason)
				assert.ErrorIs(t, err, ErrNotSatisfied, "Should return ErrNotSatisfied")
			}
		})
	}
}