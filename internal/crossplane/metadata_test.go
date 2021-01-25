package crossplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseLabels(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		want    *Labels
		wantErr bool
	}{
		{
			name:    "requires valid ServiceName",
			labels:  map[string]string{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "parses empty labels successfully",
			labels: map[string]string{
				ServiceNameLabel: string(RedisService),
			},
			want: &Labels{
				ServiceName: RedisService,
				Bindable:    true,
			},
			wantErr: false,
		},
		{
			name: "parses labels successfully",
			labels: map[string]string{
				ServiceNameLabel: string(RedisService),
				ServiceIDLabel:   "sid",
				PlanNameLabel:    "pname",
				InstanceIDLabel:  "iid",
				ParentIDLabel:    "pid",
				BindableLabel:    "false",
				UpdatableLabel:   "true",
				DeletedLabel:     "true",
			},
			want: &Labels{
				ServiceName: RedisService,
				ServiceID:   "sid",
				PlanName:    "pname",
				InstanceID:  "iid",
				ParentID:    "pid",
				Bindable:    false,
				Updatable:   true,
				Deleted:     true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLabels(tt.labels)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
