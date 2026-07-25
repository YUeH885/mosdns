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

package routecache

import (
	"context"
	"errors"
	"testing"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

func TestRouteCache(t *testing.T) {
	directCalls := 0
	direct := sequence.ExecutableFunc(func(_ context.Context, qCtx *query_context.Context) error {
		directCalls++
		qCtx.SetResponse(new(dns.Msg))
		return nil
	})
	c := newRouteCache(&Args{Size: 1024, TTL: 60})
	t.Cleanup(func() { _ = c.Close() })

	nextCalls := 0
	next := sequence.NewChainWalker([]*sequence.ChainNode{{
		E: sequence.ExecutableFunc(func(_ context.Context, qCtx *query_context.Context) error {
			nextCalls++
			SetUpstream(qCtx, direct)
			qCtx.SetResponse(new(dns.Msg))
			return nil
		}),
	}}, nil)

	for range 2 {
		q := new(dns.Msg)
		q.SetQuestion("example.com.", dns.TypeA)
		if err := c.Exec(context.Background(), query_context.NewContext(q), next); err != nil {
			t.Fatal(err)
		}
	}
	if nextCalls != 1 || directCalls != 1 {
		t.Fatalf("want one route and one direct call, got %d and %d", nextCalls, directCalls)
	}
}

func TestRouteCacheDeletesFailedRoute(t *testing.T) {
	directCalls := 0
	direct := sequence.ExecutableFunc(func(_ context.Context, _ *query_context.Context) error {
		directCalls++
		return errors.New("upstream failed")
	})
	c := newRouteCache(&Args{Size: 1024, TTL: 60})
	t.Cleanup(func() { _ = c.Close() })

	nextCalls := 0
	next := sequence.NewChainWalker([]*sequence.ChainNode{{
		E: sequence.ExecutableFunc(func(_ context.Context, qCtx *query_context.Context) error {
			nextCalls++
			if nextCalls == 1 {
				SetUpstream(qCtx, direct)
			}
			qCtx.SetResponse(new(dns.Msg))
			return nil
		}),
	}}, nil)

	for range 3 {
		q := new(dns.Msg)
		q.SetQuestion("example.com.", dns.TypeA)
		if err := c.Exec(context.Background(), query_context.NewContext(q), next); err != nil {
			t.Fatal(err)
		}
	}
	if directCalls != 1 || nextCalls != 3 {
		t.Fatalf("want one failed route call and three main calls, got %d and %d", directCalls, nextCalls)
	}
}

func TestGetKeyOnlyCachesAddressQueries(t *testing.T) {
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		q := new(dns.Msg)
		q.SetQuestion("example.com.", qtype)
		if _, ok := getKey(q); !ok {
			t.Fatalf("qtype %d should be cached", qtype)
		}
	}

	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeCNAME)
	if _, ok := getKey(q); ok {
		t.Fatal("CNAME query should not be cached")
	}
}
