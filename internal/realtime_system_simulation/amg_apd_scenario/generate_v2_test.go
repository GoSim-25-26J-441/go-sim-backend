package amg_apd_scenario

import (
	"testing"
)

func TestScenarioV2_QueueServiceValidates(t *testing.T) {
	amg := []byte(`services:
  - name: worker
    type: service
  - name: jobs
    type: queue
dependencies:
  - from: worker
    to: jobs
    kind: queue
    sync: true
configs:
  slo:
    target_rps: 10
`)
	yamlStr, err := GenerateScenarioYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ParseScenarioDocYAML([]byte(yamlStr))
	if err != nil {
		t.Fatal(err)
	}
	var q *ServiceDoc
	for i := range sc.Services {
		if sc.Services[i].ID == "jobs" {
			q = &sc.Services[i]
			break
		}
	}
	if q == nil || q.Kind != "queue" {
		t.Fatalf("jobs: want kind queue, got %#v", q)
	}
	if q.Behavior == nil || q.Behavior.Queue == nil || q.Behavior.Queue.ConsumerTarget != "worker:/read" {
		t.Fatalf("queue behavior: %#v", q.Behavior)
	}
}

func TestScenarioV2_TopicServiceValidates(t *testing.T) {
	amg := []byte(`services:
  - name: worker
    type: service
  - name: events
    type: topic
dependencies:
  - from: worker
    to: events
    kind: topic
    sync: true
configs:
  slo:
    target_rps: 10
`)
	yamlStr, err := GenerateScenarioYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ParseScenarioDocYAML([]byte(yamlStr))
	if err != nil {
		t.Fatal(err)
	}
	var top *ServiceDoc
	for i := range sc.Services {
		if sc.Services[i].ID == "events" {
			top = &sc.Services[i]
			break
		}
	}
	if top == nil || top.Kind != "topic" {
		t.Fatalf("events: want kind topic, got %#v", top)
	}
	if top.Behavior == nil || top.Behavior.Topic == nil || len(top.Behavior.Topic.Subscribers) != 1 {
		t.Fatalf("topic behavior: %#v", top.Behavior)
	}
	if top.Behavior.Topic.Subscribers[0].ConsumerTarget != "worker:/read" {
		t.Fatalf("subscriber target: %q", top.Behavior.Topic.Subscribers[0].ConsumerTarget)
	}
}

func TestTopicsArray_AddsTopicService(t *testing.T) {
	amg := []byte(`services:
  - name: app
    type: service
topics:
  - name: bus-events
dependencies: []
configs:
  slo:
    target_rps: 5
`)
	sc, _, err := GenerateFromAMGAPDYAML(amg)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range sc.Services {
		if s.ID == "bus-events" && s.Kind == "topic" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected topics[] to add service bus-events, got %#v", sc.Services)
	}
}
