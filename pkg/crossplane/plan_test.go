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
				PlanSize:  "small",
				SLA:       SLAStandard,
			},
			b: Labels{
				ServiceID: "id1",
				PlanSize:  "small",
				SLA:       SLAStandard,
			},
			want:    0,
			wantErr: nil,
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
