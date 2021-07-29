package crossplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Plan_CmpSize(t *testing.T) {
	tests := map[string]struct {
		a       Labels
		b       Labels
		want    int
		wantErr error
	}{
		"is equal": {
			a:       Labels{ServiceID: "id1", PlanSize: SizeSmall},
			b:       Labels{ServiceID: "id1", PlanSize: SizeSmall},
			want:    0,
			wantErr: nil,
		},
		"larger plan size": {
			a:       Labels{ServiceID: "id1", PlanSize: SizeSmall},
			b:       Labels{ServiceID: "id1", PlanSize: SizeMedium},
			want:    -1,
			wantErr: nil,
		},
		"smaller plan size": {
			a:       Labels{ServiceID: "id1", PlanSize: SizeMedium},
			b:       Labels{ServiceID: "id1", PlanSize: SizeXSmall},
			want:    1,
			wantErr: nil,
		},
		"unknown plan size": {
			a:       Labels{ServiceID: "id1", PlanSize: SizeSmall},
			b:       Labels{ServiceID: "id1", PlanSize: "mega-large"},
			want:    0,
			wantErr: ErrSizeUnknown,
		},
		"different services": {
			a:       Labels{ServiceID: "id1", PlanSize: SizeSmall},
			b:       Labels{ServiceID: "id2", PlanSize: SizeSmall},
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
			got, err := pa.CmpSize(pb)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_Plan_CmpSLA(t *testing.T) {
	tests := map[string]struct {
		a       Labels
		b       Labels
		want    int
		wantErr error
	}{
		"is equal": {
			a:       Labels{ServiceID: "id1", SLA: SLAStandard},
			b:       Labels{ServiceID: "id1", SLA: SLAStandard},
			want:    0,
			wantErr: nil,
		},
		"larger SLA": {
			a:       Labels{ServiceID: "id1", SLA: SLAStandard},
			b:       Labels{ServiceID: "id1", SLA: SLAPremium},
			want:    -1,
			wantErr: nil,
		},
		"smaller SLA": {
			a:       Labels{ServiceID: "id1", SLA: SLAPremium},
			b:       Labels{ServiceID: "id1", SLA: SLAStandard},
			want:    1,
			wantErr: nil,
		},
		"unknown sla": {
			a:       Labels{ServiceID: "id1", SLA: SLAStandard},
			b:       Labels{ServiceID: "id1", SLA: "99.99999"},
			want:    0,
			wantErr: ErrSLAUnknown,
		},
		"different services": {
			a:       Labels{ServiceID: "id1", SLA: SLAStandard},
			b:       Labels{ServiceID: "id2", SLA: SLAStandard},
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
			got, err := pa.CmpSLA(pb)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
