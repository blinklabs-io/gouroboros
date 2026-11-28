// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package muxer_test

import (
	"context"
	"io"
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

type shortWriteConn struct{ *mockConn }

func (*shortWriteConn) Write(b []byte) (int, error) { return len(b) - 1, nil }

func TestWriteObservationRejectsShortWriteWithoutError(t *testing.T) {
	conn := &shortWriteConn{newMockConn()}
	m := muxer.New(conn)
	defer m.Stop(context.Background())
	recorder, err := muxer.NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(muxer.WriteMetadata{ProtocolID: 3})
	require.NotNil(t, trace)
	segment := muxer.NewSegment(3, []byte{1, 2, 3}, false)
	require.NotNil(t, segment)
	segment.AddWriteSpan(trace, true, true, 0)
	require.ErrorIs(t, m.Send(segment), io.ErrShortWrite)
	record := recorder.Snapshot()[0]
	require.True(t, record.WriteCompletedAt.IsZero())
	require.Equal(t, muxer.WriteFailureTransport, record.Failure)
}

func TestWriteObservationDeadlineFailureDoesNotStartWrite(t *testing.T) {
	conn := &writeDeadlineErrorConn{
		mockConn: newMockConn(),
		setErr:   io.ErrClosedPipe,
	}
	m := muxer.New(conn)
	defer m.Stop(context.Background())
	recorder, err := muxer.NewWriteRecorder(1)
	require.NoError(t, err)
	trace := recorder.Begin(muxer.WriteMetadata{ProtocolID: 3})
	require.NotNil(t, trace)
	segment := muxer.NewSegment(3, []byte{1}, false)
	require.NotNil(t, segment)
	segment.AddWriteSpan(trace, true, true, 0)
	require.ErrorIs(t, m.Send(segment), io.ErrClosedPipe)
	record := recorder.Snapshot()[0]
	require.True(t, record.WriteStartedAt.IsZero())
	require.True(t, record.WriteCompletedAt.IsZero())
	require.Equal(t, muxer.WriteFailureTransport, record.Failure)
	require.Zero(t, conn.writeCnt)
}
