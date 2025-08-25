package specification

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Benchmark tests for performance evaluation

func BenchmarkFilter(b *testing.B) {
	users := createTestUsers()
	spec := &UserActiveSpec{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Filter(users, spec)
	}
}

func BenchmarkFilter_LargeDataset(b *testing.B) {
	// Create a larger dataset for more realistic benchmarking
	users := make([]*TestUser, 1000)
	for i := 0; i < 1000; i++ {
		users[i] = &TestUser{
			ID:       i,
			Name:     fmt.Sprintf("User%d", i),
			Age:      20 + (i % 50), // Ages 20-69
			Email:    fmt.Sprintf("user%d@example.com", i),
			IsActive: i%3 != 0, // 2/3 active
		}
	}

	spec := &UserActiveSpec{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Filter(users, spec)
	}
}

func BenchmarkAndSpecification(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	andSpec := NewAndSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 18},
		&UserActiveSpec{},
		&UserEmailValidSpec{},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = andSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkOrSpecification(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // This will fail
		&UserActiveSpec{},            // This will pass, short-circuit
		&UserEmailValidSpec{},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = orSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkOrSpecification_AllFail(b *testing.B) {
	user := &TestUser{ID: 2, Name: "Bob", Age: 17, Email: "bob@example.com", IsActive: true}
	orSpec := NewOrSpecification[TestUser](
		&UserAgeMinSpec{MinAge: 100}, // Fail
		&UserAgeMaxSpec{MaxAge: 10},  // Fail
		&AlwaysFailSpec[TestUser]{},  // Fail
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = orSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkNotSpecification(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, IsActive: true}
	notSpec := NewNotSpecification[TestUser](&UserActiveSpec{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = notSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkComplexSpecification(b *testing.B) {
	// Complex nested specification:
	// (Active AND Age 18-65) OR (Email Valid AND Age > 30)
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	
	activeAndAgeSpec := NewAndSpecification[TestUser](
		&UserActiveSpec{},
		&UserAgeMinSpec{MinAge: 18},
		&UserAgeMaxSpec{MaxAge: 65},
	)
	
	emailAndOlderSpec := NewAndSpecification[TestUser](
		&UserEmailValidSpec{},
		&UserAgeMinSpec{MinAge: 30},
	)
	
	complexSpec := NewOrSpecification[TestUser](
		activeAndAgeSpec,
		emailAndOlderSpec,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = complexSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkFilter_ComplexSpecification(b *testing.B) {
	users := createTestUsers()
	
	// Complex specification for filtering
	complexSpec := NewAndSpecification[TestUser](
		NewOrSpecification[TestUser](
			&UserAgeMinSpec{MinAge: 18},
			&UserActiveSpec{},
		),
		&UserEmailValidSpec{},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Filter(users, complexSpec)
	}
}

// Benchmark different specification combinations
func BenchmarkSpecificationComparisons(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	benchmarks := []struct {
		name string
		spec Specification[TestUser]
	}{
		{
			name: "Single",
			spec: &UserActiveSpec{},
		},
		{
			name: "AND_2",
			spec: NewAndSpecification[TestUser](
				&UserActiveSpec{},
				&UserEmailValidSpec{},
			),
		},
		{
			name: "AND_5",
			spec: NewAndSpecification[TestUser](
				&UserActiveSpec{},
				&UserEmailValidSpec{},
				&UserAgeMinSpec{MinAge: 18},
				&UserAgeMaxSpec{MaxAge: 65},
				&AlwaysPassSpec[TestUser]{},
			),
		},
		{
			name: "OR_2",
			spec: NewOrSpecification[TestUser](
				&UserActiveSpec{},
				&UserEmailValidSpec{},
			),
		},
		{
			name: "OR_5",
			spec: NewOrSpecification[TestUser](
				&AlwaysFailSpec[TestUser]{},
				&AlwaysFailSpec[TestUser]{},
				&AlwaysFailSpec[TestUser]{},
				&AlwaysFailSpec[TestUser]{},
				&UserActiveSpec{}, // This will pass last
			),
		},
		{
			name: "NOT",
			spec: NewNotSpecification[TestUser](&UserActiveSpec{}),
		},
		{
			name: "Nested_Complex",
			spec: NewOrSpecification[TestUser](
				NewAndSpecification[TestUser](
					&UserActiveSpec{},
					&UserEmailValidSpec{},
				),
				NewNotSpecification[TestUser](
					&UserAgeMinSpec{MinAge: 100},
				),
			),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = bm.spec.IsSatisfiedBy(user)
			}
		})
	}
}

// Benchmark memory allocations
func BenchmarkSpecification_Allocations(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}

	b.Run("Single_Spec", func(b *testing.B) {
		spec := &UserActiveSpec{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = spec.IsSatisfiedBy(user)
		}
	})

	b.Run("AND_Spec", func(b *testing.B) {
		andSpec := NewAndSpecification[TestUser](
			&UserActiveSpec{},
			&UserEmailValidSpec{},
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = andSpec.IsSatisfiedBy(user)
		}
	})

	b.Run("Filter", func(b *testing.B) {
		users := createTestUsers()
		spec := &UserActiveSpec{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = Filter(users, spec)
		}
	})
}

// Benchmark scaling with data size
func BenchmarkFilter_Scaling(b *testing.B) {
	spec := &UserActiveSpec{}
	
	sizes := []int{10, 100, 1000, 10000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			// Create users of specified size
			users := make([]*TestUser, size)
			for i := 0; i < size; i++ {
				users[i] = &TestUser{
					ID:       i,
					Name:     fmt.Sprintf("User%d", i),
					Age:      20 + (i % 50),
					Email:    fmt.Sprintf("user%d@example.com", i),
					IsActive: i%2 == 0, // Half active
				}
			}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Filter(users, spec)
			}
		})
	}
}

// Performance test to ensure no regression
func TestPerformanceRegression(t *testing.T) {
	// This test ensures that basic operations complete within reasonable time
	// It's not a benchmark but a regression test
	
	const iterations = 10000
	
	t.Run("Filter_Performance", func(t *testing.T) {
		users := createTestUsers()
		spec := &UserActiveSpec{}
		
		for i := 0; i < iterations; i++ {
			result, err := Filter(users, spec)
			require.Error(t, err) // Some users are inactive
			require.NotEmpty(t, result)
		}
	})
	
	t.Run("ComplexSpec_Performance", func(t *testing.T) {
		user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
		complexSpec := NewOrSpecification[TestUser](
			NewAndSpecification[TestUser](
				&UserActiveSpec{},
				&UserEmailValidSpec{},
				&UserAgeMinSpec{MinAge: 18},
			),
			NewNotSpecification[TestUser](
				&UserAgeMaxSpec{MaxAge: 10},
			),
		)
		
		for i := 0; i < iterations; i++ {
			err := complexSpec.IsSatisfiedBy(user)
			require.NoError(t, err)
		}
	})
}

// Benchmark worst-case scenarios
func BenchmarkWorstCase_DeepNesting(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	
	// Create deeply nested specification (10 levels)
	var spec Specification[TestUser] = &UserActiveSpec{}
	for i := 0; i < 10; i++ {
		spec = NewNotSpecification[TestUser](spec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spec.IsSatisfiedBy(user)
	}
}

func BenchmarkWorstCase_WideAND(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	
	// Create wide AND specification (many specs that all pass)
	specs := make([]Specification[TestUser], 50)
	for i := 0; i < 50; i++ {
		specs[i] = &AlwaysPassSpec[TestUser]{}
	}
	
	andSpec := NewAndSpecification[TestUser](specs...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = andSpec.IsSatisfiedBy(user)
	}
}

func BenchmarkWorstCase_WideOR_AllFail(b *testing.B) {
	user := &TestUser{ID: 1, Name: "Alice", Age: 25, Email: "alice@example.com", IsActive: true}
	
	// Create wide OR specification where all fail (worst case for OR)
	specs := make([]Specification[TestUser], 50)
	for i := 0; i < 50; i++ {
		specs[i] = &AlwaysFailSpec[TestUser]{Reason: fmt.Sprintf("fail%d", i)}
	}
	
	orSpec := NewOrSpecification[TestUser](specs...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = orSpec.IsSatisfiedBy(user)
	}
}