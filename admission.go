package admission

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	Name     string            `yaml:"name"`
	Validate *ValidateRule     `yaml:"validate,omitempty"`
	Mutate   *MutateRule       `yaml:"mutate,omitempty"`
}

type ValidateRule struct {
	Message       string `yaml:"message"`
	RequireLabels []string `yaml:"require_labels,omitempty"`
	RequireLimit  bool   `yaml:"require_memory_limit,omitempty"`
	DenyPrivileged bool  `yaml:"deny_privileged,omitempty"`
}

type MutateRule struct {
	AddLabels map[string]string `yaml:"add_labels,omitempty"`
}

type Config struct {
	Rules  []Rule                `yaml:"rules"`
	Quotas map[string]*Quota     `yaml:"quotas,omitempty"` // keyed by namespace name
}

type Quota struct {
	MaxMemory   string `yaml:"max_memory"`   // e.g. "8G"
	MaxCPU      string `yaml:"max_cpu"`       // e.g. "4.0"
	MaxServices int    `yaml:"max_services"`
}

type Controller struct {
	client *client.Client
	logger *slog.Logger
	config Config
}

func New(cli *client.Client, configPath string, logger *slog.Logger) *Controller {
	c := &Controller{client: cli, logger: logger}
	data, err := os.ReadFile(configPath)
	if err != nil {
		logger.Warn("no admission config, running with no rules", "path", configPath)
		return c
	}
	yaml.Unmarshal(data, &c.config)
	logger.Info("admission rules loaded", "count", len(c.config.Rules))
	return c
}

func (c *Controller) HandleEvent(ctx context.Context, event events.Message) {
	if event.Type != events.ServiceEventType || event.Action != events.ActionCreate {
		return
	}
	c.evaluate(ctx, event.Actor.ID)
}

func (c *Controller) evaluate(ctx context.Context, serviceID string) {
	svc, _, err := c.client.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
	if err != nil {
		return
	}

	needsUpdate := false
	for _, rule := range c.config.Rules {
		if rule.Validate != nil {
			if !c.validate(ctx, svc, rule) {
				c.logger.Error("admission denied", "service", svc.Spec.Name, "rule", rule.Name, "message", rule.Validate.Message)
				c.client.ServiceRemove(ctx, serviceID)
				return
			}
		}
		if rule.Mutate != nil {
			if c.mutate(&svc, rule) {
				needsUpdate = true
			}
		}
	}

	// Check namespace quotas
	if ns := svc.Spec.Labels["swarmex.namespace"]; ns != "" && len(c.config.Quotas) > 0 {
		if !c.checkQuota(ctx, svc, ns) {
			c.logger.Error("admission denied — namespace quota exceeded",
				"service", svc.Spec.Name, "namespace", ns)
			c.client.ServiceRemove(ctx, serviceID)
			return
		}
	}

	if needsUpdate {
		_, err := c.client.ServiceUpdate(ctx, serviceID, svc.Version, svc.Spec, types.ServiceUpdateOptions{})
		if err != nil {
			c.logger.Error("admission mutation failed", "service", svc.Spec.Name, "error", err)
		} else {
			c.logger.Info("admission mutation applied", "service", svc.Spec.Name)
		}
	}
}

func (c *Controller) validate(ctx context.Context, svc swarm.Service, rule Rule) bool {
	v := rule.Validate
	if v.RequireLimit {
		limits := svc.Spec.TaskTemplate.Resources
		if limits == nil || limits.Limits == nil || limits.Limits.MemoryBytes == 0 {
			return false
		}
	}
	for _, label := range v.RequireLabels {
		if _, ok := svc.Spec.Labels[strings.TrimSpace(label)]; !ok {
			return false
		}
	}
	return true
}


func (c *Controller) checkQuota(ctx context.Context, svc swarm.Service, ns string) bool {
	quota, ok := c.config.Quotas[ns]
	if !ok {
		return true // no quota for this namespace
	}

	services, err := c.client.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return true
	}

	var totalMem int64
	svcCount := 0
	for _, s := range services {
		if s.Spec.Labels["swarmex.namespace"] != ns || s.ID == svc.ID {
			continue
		}
		svcCount++
		if s.Spec.TaskTemplate.Resources != nil && s.Spec.TaskTemplate.Resources.Limits != nil {
			totalMem += s.Spec.TaskTemplate.Resources.Limits.MemoryBytes
		}
	}

	// Add the new service
	if svc.Spec.TaskTemplate.Resources != nil && svc.Spec.TaskTemplate.Resources.Limits != nil {
		totalMem += svc.Spec.TaskTemplate.Resources.Limits.MemoryBytes
	}
	svcCount++

	if quota.MaxServices > 0 && svcCount > quota.MaxServices {
		c.logger.Warn("quota exceeded: max_services", "namespace", ns, "count", svcCount, "max", quota.MaxServices)
		return false
	}
	if quota.MaxMemory != "" {
		maxBytes := parseMemory(quota.MaxMemory)
		if maxBytes > 0 && totalMem > maxBytes {
			c.logger.Warn("quota exceeded: max_memory", "namespace", ns,
				"total_mb", totalMem/(1024*1024), "max_mb", maxBytes/(1024*1024))
			return false
		}
	}
	return true
}

func parseMemory(s string) int64 {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}
	multiplier := int64(1)
	switch s[len(s)-1] {
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n * multiplier
}
func (c *Controller) mutate(svc *swarm.Service, rule Rule) bool {
	m := rule.Mutate
	if m == nil || len(m.AddLabels) == 0 {
		return false
	}
	changed := false
	if svc.Spec.Labels == nil {
		svc.Spec.Labels = make(map[string]string)
	}
	for k, v := range m.AddLabels {
		if svc.Spec.Labels[k] != v {
			svc.Spec.Labels[k] = v
			changed = true
		}
	}
	return changed
}
