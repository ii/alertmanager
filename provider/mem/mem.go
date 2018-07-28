// Copyright 2016 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/lic:wenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mem

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

// Alerts gives access to a set of alerts. All methods are goroutine-safe.
type Alerts struct {
	alerts store.Store
	cancel context.CancelFunc

	mtx       sync.Mutex
	listeners map[int]chan *types.Alert
	next      int
}

// NewAlerts returns a new alert provider.
func NewAlerts(ctx context.Context, m types.Marker, intervalGC time.Duration) (*Alerts, error) {
	ctx, cancel := context.WithCancel(ctx)
	a := &Alerts{
		alerts:    store.NewAlerts(ctx, intervalGC),
		cancel:    cancel,
		listeners: map[int]chan *types.Alert{},
		next:      0,
	}
	a.alerts.SetGCCallback(func(alert *types.Alert) {
		m.Delete(alert.Fingerprint())
	})

	return a, nil
}

// Close the alert provider.
func (a *Alerts) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// Subscribe returns an iterator over active alerts that have not been
// resolved and successfully notified about.
// They are not guaranteed to be in chronological order.
func (a *Alerts) Subscribe() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)
	a.mtx.Lock()
	i := a.next
	a.next++
	a.listeners[i] = ch
	a.mtx.Unlock()

	go func() {
		defer func() {
			a.mtx.Lock()
			delete(a.listeners, i)
			close(ch)
			a.mtx.Unlock()
		}()

		for a := range a.alerts.List() {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}

		<-done
	}()

	return provider.NewAlertIterator(ch, done, nil)
}

// GetPending returns an iterator over all alerts that have
// pending notifications.
func (a *Alerts) GetPending() provider.AlertIterator {
	var (
		ch   = make(chan *types.Alert, 200)
		done = make(chan struct{})
	)

	go func() {
		defer close(ch)

		for a := range a.alerts.List() {
			select {
			case ch <- a:
			case <-done:
				return
			}
		}
	}()

	return provider.NewAlertIterator(ch, done, nil)
}

// Get returns the alert for a given fingerprint.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	return a.alerts.Get(fp)
}

// Put adds the given alert to the set.
func (a *Alerts) Put(alerts ...*types.Alert) error {
	a.mtx.Lock()
	listeners := make([]chan *types.Alert, 0, len(a.listeners))
	for _, ch := range a.listeners {
		listeners = append(listeners, ch)
	}
	a.mtx.Unlock()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		if old, err := a.alerts.Get(fp); err == nil {
			// Merge alerts if there is an overlap in activity range.
			if (alert.EndsAt.After(old.StartsAt) && alert.EndsAt.Before(old.EndsAt)) ||
				(alert.StartsAt.After(old.StartsAt) && alert.StartsAt.Before(old.EndsAt)) {
				alert = old.Merge(alert)
			}
		}

		if err := a.alerts.Set(alert); err != nil {
			// TODO: Log something??
		}

		for _, ch := range listeners {
			ch <- alert
		}
	}

	return nil
}
