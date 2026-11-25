package k8s

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) notifyNetworkChanged() {
	if c.networkNotifyCh == nil {
		return
	}
	select {
	case c.networkNotifyCh <- struct{}{}:
	default:
	}
}

func (c *Controller) setupNetworkHandler(ctx context.Context, handlerFunc RefreshNetworkFunc) {
	if handlerFunc == nil {
		return
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { c.notifyNetworkChanged() },
		UpdateFunc: func(any, any) { c.notifyNetworkChanged() },
		DeleteFunc: func(any) { c.notifyNetworkChanged() },
	}
	if c.serviceInformer != nil {
		_, _ = c.serviceInformer.Informer().AddEventHandler(handlers)
	}
	if c.endpointSliceInformer != nil {
		_, _ = c.endpointSliceInformer.Informer().AddEventHandler(handlers)
	}
	if c.ingressInformer != nil {
		_, _ = c.ingressInformer.Informer().AddEventHandler(handlers)
	}

	refresh := func() {
		snap, err := c.GetNetworkSnapshot(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Debug("network refresh skipped", "error", err)
			}
			return
		}
		if err := handlerFunc(ctx, snap); err != nil && ctx.Err() == nil {
			slog.Debug("network refresh handler failed", "error", err)
		}
	}

	go func() {
		refresh()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			case <-c.networkNotifyCh:
				refresh()
			}
		}
	}()
}

// GetNetworkSnapshot reads Services, EndpointSlices, and Ingresses from informer caches
// and joins node network throughput and drop/error rates when available.
func (c *Controller) GetNetworkSnapshot(ctx context.Context) (model.NetworkSnapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return model.NetworkSnapshot{}, ctx.Err()
	}

	type epCount struct {
		ready int
		total int
	}
	epByService := make(map[string]epCount)
	hasEndpointSlices := false

	if c.endpointSliceInformer != nil {
		slicesList, err := c.endpointSliceInformer.Lister().List(labels.Everything())
		if err == nil {
			hasEndpointSlices = true
			type epAddrKey struct {
				svc  string
				addr string
			}
			seenReady := make(map[epAddrKey]bool)
			for _, es := range slicesList {
				if es == nil {
					continue
				}
				svcName := es.Labels[discoveryv1.LabelServiceName]
				if svcName == "" {
					continue
				}
				svcKey := es.Namespace + "/" + svcName
				for _, ep := range es.Endpoints {
					if len(ep.Addresses) == 0 {
						continue
					}
					addrKey := epAddrKey{svc: svcKey, addr: ep.Addresses[0]}
					ready := ep.Conditions.Ready == nil || *ep.Conditions.Ready
					prevReady, seen := seenReady[addrKey]
					if !seen {
						seenReady[addrKey] = ready
					} else if ready && !prevReady {
						seenReady[addrKey] = true
					}
				}
			}
			for addrKey, ready := range seenReady {
				cnt := epByService[addrKey.svc]
				cnt.total++
				if ready {
					cnt.ready++
				}
				epByService[addrKey.svc] = cnt
			}
		}
	}

	var svcs []model.ServiceModel
	if c.serviceInformer != nil {
		rawSvcs, err := c.serviceInformer.Lister().List(labels.Everything())
		if err != nil {
			return model.NetworkSnapshot{}, err
		}
		svcs = make([]model.ServiceModel, 0, len(rawSvcs))
		for _, svc := range rawSvcs {
			if svc == nil {
				continue
			}
			cnt := epByService[svc.Namespace+"/"+svc.Name]
			svcs = append(svcs, model.NewServiceModel(svc, cnt.ready, cnt.total, hasEndpointSlices))
		}
	}
	model.SortServiceModelsBy(svcs, "NS", true)

	var ings []model.IngressModel
	if c.ingressInformer != nil {
		rawIngs, err := c.ingressInformer.Lister().List(labels.Everything())
		if err == nil {
			ings = make([]model.IngressModel, 0, len(rawIngs))
			for _, ing := range rawIngs {
				if ing == nil {
					continue
				}
				ings = append(ings, model.NewIngressModel(ing))
			}
		}
	}
	model.SortIngressModelsBy(ings, "NS", true)

	var (
		perNodeErrors map[string]metrics.NetworkErrorRates
		hasMetrics    bool
		nodeNetwork   []model.NodeNetworkModel
	)
	if np, ok := c.metricsSource.(metrics.NetworkErrorsProvider); ok {
		if _, pn, err := np.GetClusterNetworkErrorRates(ctx); err == nil {
			perNodeErrors = pn
			hasMetrics = true
		}
	}

	if c.metricsSource != nil && c.nodeInformer != nil {
		if nodes, err := c.nodeInformer.Lister().List(labels.Everything()); err == nil {
			seenNodes := make(map[string]bool, len(nodes))
			for _, node := range nodes {
				if node == nil {
					continue
				}
				seenNodes[node.Name] = true
				nn := model.NodeNetworkModel{NodeName: node.Name}
				if nm, mErr := c.metricsSource.GetNodeMetrics(ctx, node.Name); mErr == nil && nm != nil {
					nn.RxRateBps = nm.NetworkRxRate
					nn.TxRateBps = nm.NetworkTxRate
					hasMetrics = true
				}
				if er, ok := perNodeErrors[node.Name]; ok {
					nn.RxDroppedSec = er.RxDroppedPerSec
					nn.TxDroppedSec = er.TxDroppedPerSec
					nn.RxErrorsSec = er.RxErrorsPerSec
					nn.TxErrorsSec = er.TxErrorsPerSec
				}
				nodeNetwork = append(nodeNetwork, nn)
			}
			for nodeName, er := range perNodeErrors {
				if !seenNodes[nodeName] {
					nodeNetwork = append(nodeNetwork, model.NodeNetworkModel{
						NodeName:     nodeName,
						RxDroppedSec: er.RxDroppedPerSec,
						TxDroppedSec: er.TxDroppedPerSec,
						RxErrorsSec:  er.RxErrorsPerSec,
						TxErrorsSec:  er.TxErrorsPerSec,
					})
				}
			}
		}
	}
	slices.SortFunc(nodeNetwork, func(a, b model.NodeNetworkModel) int {
		return strings.Compare(a.NodeName, b.NodeName)
	})

	summary := model.ComputeNetworkSummary(svcs, ings, nodeNetwork, hasMetrics)
	return model.NetworkSnapshot{
		Summary:     summary,
		Services:    svcs,
		Ingresses:   ings,
		NodeNetwork: nodeNetwork,
	}, nil
}
