/*
 * Copyright (C) 2020-2022, IrineSistiana
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

package drop_resp

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

const PluginType = "drop_resp"

var droppedMark = query_context.RegKey()

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*DropResp)(nil)

type DropResp struct{}

func QuickSetup(_ sequence.BQ, _ string) (any, error) {
	return &DropResp{}, nil
}

func (b *DropResp) Exec(_ context.Context, qCtx *query_context.Context) error {
	qCtx.SetResponse(nil)
	qCtx.SetMark(droppedMark)
	return nil
}

// IsDropped 返回当前响应是否被 drop_resp 丢弃。
func IsDropped(qCtx *query_context.Context) bool {
	return qCtx.HasMark(droppedMark)
}

// ClearDropped 清除之前留下的 drop_resp 标记。
func ClearDropped(qCtx *query_context.Context) {
	qCtx.DeleteMark(droppedMark)
}
