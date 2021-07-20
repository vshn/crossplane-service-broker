package crossplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Plan_Cmp(t *testing.T) {
	tests := map[string]struct {
		a       Labels
		b       Labels
		want    int
		wantErr error
	}{
		"is equal": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			want:    0,
			wantErr: nil,
		},
		"larger SLA": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAPremium,
			},
			want:    0,
			wantErr: nil,
		},
		"smaller SLA": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAPremium,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			want:    0,
			wantErr: nil,
		},
		"larger plan size": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeMedium,
				SLA:       SLAStandard,
			},
			want:    -1,
			wantErr: nil,
		},
		"smaller plan size": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeMedium,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeXSmall,
				SLA:       SLAStandard,
			},
			want:    1,
			wantErr: nil,
		},
		"unknown plan size": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  "mega-large",
				SLA:       SLAStandard,
			},
			want:    0,
			wantErr: ErrSizeUnknown,
		},
		"unknown sla": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       "99.99999",
			},
			want:    0,
			wantErr: ErrSLAUnknown,
		},
		"different services": {
			a: Labels{
				ServiceID: "id1",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id2",
				PlanSize:  SizeSmall,
				SLA:       SLAStandard,
			},
			want:    0,
			wantErr: ErrDifferentService,
		},
	}
	for k, tc := range tests {
		t.Run(k, func(t *testing.T) {
			pa := Plan{
				Labels: &tc.a,
			}
			pb := Plan{
				Labels: &tc.b,
			}
			got, err := pa.Cmp(pb)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
