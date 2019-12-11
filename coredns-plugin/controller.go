package egressoperator

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/rewrite"
	api "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type dnsControl struct {
	reflector *cache.Reflector

	// stopLock is used to enforce only a single call to Stop is active.
	// Needed because we allow stopping through an http endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock sync.Mutex
	shutdown bool
	stopCh   chan struct{}

	ready     chan struct{}
	readyOnce sync.Once
}

// newDNSController creates a controller for CoreDNS.
func newdnsController(kubeClient kubernetes.Interface, namespace, zone string, rulesCallback func([]rewrite.Rule)) *dnsControl {
	dns := &dnsControl{
		stopCh: make(chan struct{}),
		ready:  make(chan struct{}),
	}

	store := cache.NewUndeltaStore(func(is []interface{}) {
		dns.readyOnce.Do(func() {
			close(dns.ready)
		})

		rules := make([]rewrite.Rule, 0, len(is))

		for _, i := range is {
			svc := i.(*api.Service)
			if svc == nil {
				continue
			}

			from, ok := svc.Annotations["egress.monzo.com/dns-name"]
			if !ok {
				log.Warningf("%s is missing dns-name annotation", svc.Name)
				continue
			}

			to := fmt.Sprintf("%s.%s.svc.%s", svc.Name, svc.Namespace, zone)

			rewriteQuestionFrom := plugin.Name(from).Normalize()
			rewriteQuestionTo := plugin.Name(to).Normalize()

			rewriteAnswerFromPattern, err := regexp.Compile(rewriteQuestionTo)
			if err != nil {
				continue
			}

			rules = append(rules, &exactNameRule{
				NextAction: "stop",
				From:       rewriteQuestionFrom,
				To:         rewriteQuestionTo,
				ResponseRule: rewrite.ResponseRule{
					Active:      true,
					Type:        "name",
					Pattern:     rewriteAnswerFromPattern,
					Replacement: rewriteQuestionFrom,
				},
			})
		}

		rulesCallback(rules)
	}, cache.MetaNamespaceKeyFunc)

	s := labels.SelectorFromSet(map[string]string{"app": "egress-gateway"})

	dns.reflector = cache.NewReflector(&cache.ListWatch{
		ListFunc:  serviceListFunc(kubeClient, namespace, s),
		WatchFunc: serviceWatchFunc(kubeClient, namespace, s),
	}, &api.Service{}, store, 0)

	return dns
}

func serviceListFunc(c kubernetes.Interface, ns string, s labels.Selector) func(meta.ListOptions) (runtime.Object, error) {
	return func(opts meta.ListOptions) (runtime.Object, error) {
		if s != nil {
			opts.LabelSelector = s.String()
		}
		listV1, err := c.CoreV1().Services(ns).List(opts)
		return listV1, err
	}
}

func serviceWatchFunc(c kubernetes.Interface, ns string, s labels.Selector) func(options meta.ListOptions) (watch.Interface, error) {
	return func(options meta.ListOptions) (watch.Interface, error) {
		if s != nil {
			options.LabelSelector = s.String()
		}
		w, err := c.CoreV1().Services(ns).Watch(options)
		return w, err
	}
}

// Stop stops the  controller.
func (dns *dnsControl) Stop() error {
	dns.stopLock.Lock()
	defer dns.stopLock.Unlock()

	// Only try draining the workqueue if we haven't already.
	if !dns.shutdown {
		close(dns.stopCh)
		dns.shutdown = true

		return nil
	}

	return fmt.Errorf("shutdown already in progress")
}

// Run starts the controller.
func (dns *dnsControl) Run() {
	go dns.reflector.Run(dns.stopCh)
	<-dns.stopCh
}
