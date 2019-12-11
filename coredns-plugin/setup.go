package egressoperator

import (
	"fmt"
	"time"

	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var log = clog.NewWithPlugin("egressoperator")

// init registers this plugin.
func init() { plugin.Register("egressoperator", setup) }

// setup is the function that gets called when the config parser see the token "egressoperator".
func setup(c *caddy.Controller) error {
	args := c.RemainingArgs()

	var config *rest.Config
	var err error
	for c.NextBlock() {
		if c.Val() == "kubeconfig" {
			args := c.RemainingArgs()
			if len(args) == 2 {
				config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
					&clientcmd.ClientConfigLoadingRules{ExplicitPath: args[0]},
					&clientcmd.ConfigOverrides{CurrentContext: args[1]},
				).ClientConfig()
				if err != nil {
					return err
				}
				break
			}
			return c.ArgErr()
		}
	}
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}

	if len(args) < 3 {
		return fmt.Errorf("must provide args in format 'egressoperator yournamespace cluster.local', got %v", args)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	o := &EgressOperator{}

	controller := newdnsController(client, args[1], args[2], o.setRules)

	c.OnStartup(func() error {
		go controller.Run()

		select {
		case <-controller.ready:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for egressoperator controller to sync")
		}

		return nil
	})

	c.OnShutdown(func() error {
		return controller.Stop()
	})

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		o.Next = next
		return o
	})

	return nil
}
