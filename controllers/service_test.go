package controllers

import (
	"reflect"
	"testing"

	v1 "github.com/monzo/egress-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_service(t *testing.T) {
	tests := []struct {
		name         string
		hijack       bool
		ready        bool
		currentState string
		wantState    string
	}{
		{
			"none",
			false,
			false,
			"",
			"false",
		},
		{
			"disable",
			false,
			false,
			"true",
			"false",
		},
		{
			"creation-notready",
			true,
			false,
			"",
			"waiting-for-pods",
		},
		{
			"creation-ready",
			true,
			true,
			"",
			"true",
		},
		{
			"enable-notready",
			true,
			false,
			"false",
			"waiting-for-pods",
		},
		{
			"enable-ready",
			true,
			true,
			"false",
			"true",
		},
		{
			"stay-waiting",
			true,
			false,
			"waiting-for-pods",
			"waiting-for-pods",
		},
		{
			"become-ready",
			true,
			true,
			"waiting-for-pods",
			"true",
		},
		{
			"stay-hijack-ready",
			true,
			true,
			"true",
			"true",
		},
		{
			"stay-hijack-notready",
			true,
			false,
			"true",
			"true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &v1.ExternalService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "google",
				},
				Spec: v1.ExternalServiceSpec{
					HijackDns: tt.hijack,
					DnsName:   "google.com",
					Ports: []v1.ExternalServicePort{
						{Port: 443},
					},
				},
			}
			var current *corev1.Service
			if tt.currentState != "" {
				current = &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"egress.monzo.com/hijack-dns": tt.currentState},
					},
				}
			}

			if got := service(es, tt.ready, current); !reflect.DeepEqual(got.Labels["egress.monzo.com/hijack-dns"], tt.wantState) {
				t.Errorf("service() state = %v, want %v", got.Labels["egress.monzo.com/hijack-dns"], tt.wantState)
			}
		})
	}
}
