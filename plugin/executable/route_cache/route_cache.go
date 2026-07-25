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
	"hash/maphash"
	"strconv"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/cache"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "route_cache"

var upstreamKey = query_context.RegKey()

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.RecursiveExecutable = (*RouteCache)(nil)

type Args struct {
	Size int `yaml:"size"`
	TTL  int `yaml:"ttl"`
}

func (a *Args) init() {
	utils.SetDefaultUnsignNum(&a.Size, 1024)
	utils.SetDefaultUnsignNum(&a.TTL, 300)
}

type RouteCache struct {
	backend *cache.Cache[key, sequence.Executable]
	ttl     time.Duration
}

func Init(_ *coremain.BP, args any) (any, error) {
	a := args.(*Args)
	a.init()
	return newRouteCache(a), nil
}

func newRouteCache(args *Args) *RouteCache {
	return &RouteCache{
		backend: cache.New[key, sequence.Executable](cache.Opts{Size: args.Size}),
		ttl:     time.Duration(args.TTL) * time.Second,
	}
}

func (c *RouteCache) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	k, ok := getKey(qCtx.Q())
	if !ok {
		return next.ExecNext(ctx, qCtx)
	}
	if upstream, expirationTime, hit := c.backend.Get(k); hit {
		if err := upstream.Exec(ctx, qCtx); err == nil {
			return nil
		}
		c.backend.CompareAndDel(k, expirationTime)
	}

	if err := next.ExecNext(ctx, qCtx); err != nil {
		return err
	}
	if upstream := GetUpstream(qCtx); upstream != nil && qCtx.R() != nil {
		c.backend.Store(k, upstream, time.Now().Add(c.ttl))
	}
	return nil
}

func (c *RouteCache) Close() error {
	return c.backend.Close()
}

func SetUpstream(qCtx *query_context.Context, upstream sequence.Executable) {
	if upstream != nil {
		qCtx.StoreValue(upstreamKey, upstream)
	}
}

func GetUpstream(qCtx *query_context.Context) sequence.Executable {
	upstream, _ := qCtx.GetValue(upstreamKey)
	v, _ := upstream.(sequence.Executable)
	return v
}

type key string

var keySeed = maphash.MakeSeed()

func (k key) Sum() uint64 {
	return maphash.String(keySeed, string(k))
}

func getKey(q *dns.Msg) (key, bool) {
	if len(q.Question) != 1 {
		return "", false
	}
	question := q.Question[0]
	if question.Qtype != dns.TypeA && question.Qtype != dns.TypeAAAA {
		return "", false
	}
	return key(strings.ToLower(question.Name) + "\x00" + strconv.Itoa(int(question.Qtype)) + "\x00" + strconv.Itoa(int(question.Qclass))), true
}
