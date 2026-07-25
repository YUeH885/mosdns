/*
 * Copyright (C) 2026
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package fallback

import (
	"context"
	"testing"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	dropresp "github.com/IrineSistiana/mosdns/v5/plugin/executable/drop_resp"
	routecache "github.com/IrineSistiana/mosdns/v5/plugin/executable/route_cache"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

func TestFallbackPrefersPrimaryBeforeThreshold(t *testing.T) {
	secondaryReady := make(chan struct{})
	var primary sequence.ExecutableFunc
	primary = func(_ context.Context, qCtx *query_context.Context) error {
		<-secondaryReady
		time.Sleep(time.Millisecond)
		routecache.SetUpstream(qCtx, primary)
		r := new(dns.Msg)
		r.Rcode = dns.RcodeSuccess
		qCtx.SetResponse(r)
		return nil
	}
	var secondary sequence.ExecutableFunc
	secondary = func(_ context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, secondary)
		r := new(dns.Msg)
		r.Rcode = dns.RcodeNameError
		qCtx.SetResponse(r)
		close(secondaryReady)
		return nil
	}
	f := fallback{
		primary:              primary,
		secondary:            secondary,
		fastFallbackDuration: time.Hour,
		alwaysStandby:        true,
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	qCtx := query_context.NewContext(q)
	if err := f.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}
	if qCtx.R().Rcode != dns.RcodeSuccess {
		t.Fatal("secondary response won before fallback threshold")
	}
	cached := routecache.GetUpstream(qCtx)
	if cached == nil {
		t.Fatal("primary upstream was not cached")
	}
	probe := query_context.NewContext(q.Copy())
	if err := cached.Exec(context.Background(), probe); err != nil {
		t.Fatal(err)
	}
	if probe.R().Rcode != dns.RcodeSuccess {
		t.Fatal("secondary upstream was cached before fallback threshold")
	}
}

func TestFallbackDoesNotCacheSecondaryRouteWithInheritedDroppedMark(t *testing.T) {
	dropper := new(dropresp.DropResp)
	var primary sequence.ExecutableFunc
	primary = func(_ context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, primary)
		qCtx.SetResponse(nil)
		return nil
	}
	var secondary sequence.ExecutableFunc
	secondary = func(_ context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, secondary)
		qCtx.SetResponse(new(dns.Msg))
		return nil
	}
	f := fallback{
		primary:              primary,
		secondary:            secondary,
		fastFallbackDuration: time.Hour,
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	qCtx := query_context.NewContext(q)
	if err := dropper.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}
	if err := f.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}
	if routecache.GetUpstream(qCtx) != nil {
		t.Fatal("secondary upstream was cached from inherited drop_resp mark")
	}
}

func TestFallbackCachesSecondaryRouteAfterDroppedPrimaryResponse(t *testing.T) {
	dropper := new(dropresp.DropResp)
	var primary sequence.ExecutableFunc
	primary = func(ctx context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, primary)
		return dropper.Exec(ctx, qCtx)
	}
	var secondary sequence.ExecutableFunc
	secondary = func(_ context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, secondary)
		qCtx.SetResponse(new(dns.Msg))
		return nil
	}
	f := fallback{
		primary:              primary,
		secondary:            secondary,
		fastFallbackDuration: time.Hour,
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	qCtx := query_context.NewContext(q)
	if err := f.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}
	if routecache.GetUpstream(qCtx) == nil {
		t.Fatal("secondary upstream was not cached after primary response was dropped")
	}
}

func TestFallbackDoesNotCacheSecondaryRouteAfterTimeout(t *testing.T) {
	releasePrimary := make(chan struct{})
	defer close(releasePrimary)
	primary := sequence.ExecutableFunc(func(_ context.Context, qCtx *query_context.Context) error {
		<-releasePrimary
		return nil
	})
	var secondary sequence.ExecutableFunc
	secondary = func(_ context.Context, qCtx *query_context.Context) error {
		routecache.SetUpstream(qCtx, secondary)
		qCtx.SetResponse(new(dns.Msg))
		return nil
	}
	f := fallback{
		primary:              primary,
		secondary:            secondary,
		fastFallbackDuration: time.Millisecond,
		alwaysStandby:        true,
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeA)
	qCtx := query_context.NewContext(q)
	if err := f.Exec(context.Background(), qCtx); err != nil {
		t.Fatal(err)
	}
	if routecache.GetUpstream(qCtx) != nil {
		t.Fatal("secondary upstream was cached after primary timeout")
	}
}
