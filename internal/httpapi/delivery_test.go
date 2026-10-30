package httpapi

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ai-usage-tracker/internal/contract"
	"ai-usage-tracker/internal/push"
	"ai-usage-tracker/internal/store"
)

func TestDeliveryRetriesWhenTheClockArrives(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	sender.FailTemporary(1)
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("enviou na falha")
	}
	rows, err := db.ListDeliveries(10)
	if err != nil || len(rows) != 1 || rows[0].Status != "pending" || rows[0].Attempts != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	api.Now = func() time.Time { return now.Add(30 * time.Second) }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("adiantou a tentativa")
	}
	api.Now = func() time.Time { return rows[0].NextAttempt }
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 1 {
		t.Fatalf("envios %d", len(sender.Payloads()))
	}
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 1 {
		t.Fatal("duplicou o aviso")
	}
}

func TestDeliveryStopsAfterTheAttemptBudget(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	if _, err := db.InsertDelivery(store.Delivery{
		ID: "dly_orcamento", IdempotencyKey: "orcamento", SubscriptionID: "sub_um",
		Kind: "threshold", Provider: "claude", Title: "t", Body: "b", Status: "pending",
		Attempts: 4, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	sender.FailTemporary(1)
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeliveries(10)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Detail != "SEND_FAILED" || rows[0].Attempts != 5 {
		t.Fatalf("%+v %v", rows, err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("enviou depois do limite")
	}
}

func TestDeliveryExpiresADeadSubscription(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	sender.FailEndpoint("https://push.example.test/one")
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeliveries(10)
	if err != nil || len(rows) != 1 || rows[0].Status != "subscription_expired" {
		t.Fatalf("%+v %v", rows, err)
	}
	subs, err := db.ListSubscriptions()
	if err != nil || len(subs) != 1 || subs[0].ExpiredAt == nil {
		t.Fatalf("%+v %v", subs, err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("enviou para inscrição morta")
	}
}

func TestDeliveryRejectsWithoutRetry(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	sender.FailRejected()
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeliveries(10)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Attempts != 1 {
		t.Fatalf("%+v %v", rows, err)
	}
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("repetiu recusa")
	}
}

func TestDeliveryKeepsAFullQueueVisible(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	for i := 0; i < deliveryQueueCap; i++ {
		_, err := db.InsertDelivery(store.Delivery{
			ID: fmt.Sprintf("dly_%02d", i), IdempotencyKey: fmt.Sprintf("ocupada-%02d", i), SubscriptionID: "sub_um",
			Kind: "threshold", Provider: "claude", Title: "t", Body: "b", Status: "pending",
			CreatedAt: now, UpdatedAt: now, NextAttempt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := api.planAlerts(alertSample(now)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeliveries(deliveryQueueCap + 5)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.Detail == "QUEUE_FULL" && row.Status == "failed" {
			found = true
		}
	}
	if !found {
		t.Fatal("fila cheia ficou invisível")
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("fila cheia enviou")
	}
}

func TestDeliveryFlushesOnlyAPage(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	for i := 0; i < deliveryFlushLimit+1; i++ {
		_, err := db.InsertDelivery(store.Delivery{
			ID: fmt.Sprintf("dly_pagina_%02d", i), IdempotencyKey: fmt.Sprintf("pagina-%02d", i), SubscriptionID: "sub_um",
			Kind: "threshold", Provider: "claude", Title: "t", Body: "b", Status: "pending",
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	if len(sender.Payloads()) != deliveryFlushLimit {
		t.Fatalf("envios %d", len(sender.Payloads()))
	}
	pending, err := db.CountPendingDeliveries()
	if err != nil || pending != 1 {
		t.Fatalf("pendentes %d %v", pending, err)
	}
}

func TestDeliveryExpiresAnOldNotice(t *testing.T) {
	db, api, sender, now := openDelivery(t)
	created := now.Add(-(deliveryMaxAge + time.Minute))
	if _, err := db.InsertDelivery(store.Delivery{
		ID: "dly_velho", IdempotencyKey: "velho", SubscriptionID: "sub_um",
		Kind: "threshold", Provider: "claude", Title: "t", Body: "b", Status: "pending",
		CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if err := api.FlushDue(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDeliveries(5)
	if err != nil || len(rows) != 1 || rows[0].Status != "failed" || rows[0].Detail != "DELIVERY_EXPIRED" {
		t.Fatalf("%+v %v", rows, err)
	}
	if len(sender.Payloads()) != 0 {
		t.Fatal("enviou aviso vencido")
	}
}

func openDelivery(t *testing.T) (*store.Store, *Server, *push.Recording, time.Time) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "tracker.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Setup("hash-op", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertSubscription(store.PushSubscription{
		ID: "sub_um", Endpoint: "https://push.example.test/one", P256dh: "chave", Auth: "auth", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	sender := push.NewRecording("key")
	api := &Server{Store: db, Now: func() time.Time { return now }, Sender: sender}
	return db, api, sender, now
}

func alertSample(now time.Time) contract.Sample {
	return contract.Sample{
		SampleID:    "smp_aviso",
		Provider:    contract.ProviderClaude,
		CollectedAt: now,
		Windows: []contract.Window{{
			Name: "five_hour", Used: 90, Limit: 100, Unit: "percent", ResetsAt: now.Add(5 * time.Hour),
		}},
	}
}
