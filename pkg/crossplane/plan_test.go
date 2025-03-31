package crossplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PlanComparer(t *testing.T) {
	type in struct {
		a    Labels
		b    Labels
		want bool
	}
	type testCase struct {
		sizeRule string
		slaRule  string
		in       map[string]in
	}
	tests := map[string]testCase{
		"empty": {
			sizeRule: "",
			slaRule:  "",
			in: map[string]in{
				"is equal": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"larger plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeMedium, SLA: SLAStandard},
					want: false,
				},
				"higher SLA": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAPremium},
					want: false,
				},
				"different services": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id2", PlanSize: SizeSmall, SLA: SLAStandard},
					want: false,
				},
			},
		},
		"SLA change": {
			sizeRule: "",
			slaRule:  "standard>premium|premium>standard",
			in: map[string]in{
				"is equal": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"larger plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeMedium, SLA: SLAStandard},
					want: false,
				},
				"higher SLA": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAPremium},
					want: true,
				},
				"lower SLA": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAPremium},
					want: true,
				},
			},
		},
		"small upgrade": {
			sizeRule: "xsmall>small",
			slaRule:  "standard>premium|premium>standard",
			in: map[string]in{
				"is equal": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"larger plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"large plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeLarge, SLA: SLAStandard},
					want: false,
				},
				"smaller plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					want: false,
				},
				"SLA and size change": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAPremium},
					want: false,
				},
			},
		},
		"larger upgrades": {
			sizeRule: "xsmall>small|xsmall>medium|xsmall>large|small>medium|small>large",
			slaRule:  "standard>premium|premium>standard",
			in: map[string]in{
				"is equal": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"larger plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					want: true,
				},
				"large plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeLarge, SLA: SLAStandard},
					want: true,
				},
				"smaller plan size": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					want: false,
				},
				"SLA and size change": {
					a:    Labels{ServiceID: "id1", PlanSize: SizeXSmall, SLA: SLAStandard},
					b:    Labels{ServiceID: "id1", PlanSize: SizeSmall, SLA: SLAPremium},
					want: false,
				},
			},
		},
	}
	for rule, tc := range tests {
		t.Run(rule, func(t *testing.T) {
			for k, in := range tc.in {
				t.Run(k, func(t *testing.T) {
					assert.NotNil(t, tc)
					pc, err := ParsePlanUpdateRules(tc.sizeRule, tc.slaRule)
					require.NoError(t, err)
					ok := pc.AllowUpdate(
						Plan{Labels: &in.a},
						Plan{Labels: &in.b},
					)
					assert.Equal(t, in.want, ok)
				})
			}
		})
	}
}

func TestPlanRedis72Changed(t *testing.T) {
	tests := []struct {
		desc     string
		a        Plan
		b        Plan
		expected bool
	}{
		{
			desc:     "Both nil labels",
			a:        Plan{Labels: nil},
			b:        Plan{Labels: nil},
			expected: false,
		},
		{
			desc:     "First plan has nil labels, second has non-7.2",
			a:        Plan{Labels: nil},
			b:        Plan{Labels: &Labels{Version: "7.0"}},
			expected: false,
		},
		{
			desc:     "First plan has nil labels, second has 7.2",
			a:        Plan{Labels: nil},
			b:        Plan{Labels: &Labels{Version: "7.2"}},
			expected: false,
		},
		{
			desc:     "First has non-7.2, second nil labels",
			a:        Plan{Labels: &Labels{Version: "7.0"}},
			b:        Plan{Labels: nil},
			expected: false,
		},
		{
			desc:     "First has 7.2, second nil labels",
			a:        Plan{Labels: &Labels{Version: "7.2"}},
			b:        Plan{Labels: nil},
			expected: false,
		},
		{
			desc:     "Both have the same version 7.2",
			a:        Plan{Labels: &Labels{Version: "7.2"}},
			b:        Plan{Labels: &Labels{Version: "7.2"}},
			expected: false,
		},
		{
			desc:     "Both have the same non-7.2 version",
			a:        Plan{Labels: &Labels{Version: "7.0"}},
			b:        Plan{Labels: &Labels{Version: "7.0"}},
			expected: false,
		},
		{
			desc:     "First has 7.2, second has different version",
			a:        Plan{Labels: &Labels{Version: "7.2"}},
			b:        Plan{Labels: &Labels{Version: "7.0"}},
			expected: true,
		},
		{
			desc:     "First has non-7.2, second has 7.2",
			a:        Plan{Labels: &Labels{Version: "7.0"}},
			b:        Plan{Labels: &Labels{Version: "7.2"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := planRedis72Changed(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.desc, tt.expected, result)
			}
		})
	}
}
