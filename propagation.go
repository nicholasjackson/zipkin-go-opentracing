// Copyright 2019 The OpenZipkin Authors
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

package zipkintracer

import (
	"net/http"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/openzipkin/zipkin-go/model"
	"github.com/openzipkin/zipkin-go/propagation"
	"github.com/openzipkin/zipkin-go/propagation/b3"
)

// DelegatingCarrier is a flexible carrier interface which can be implemented
// by types which have a means of storing the trace metadata and already know
// how to serialize themselves
type DelegatingCarrier interface {
	State() (model.SpanContext, error)
	SetState(model.SpanContext) error
}

type textMapPropagator struct {
	tracer *tracerImpl
}

func (p *textMapPropagator) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	// native zipkin-go injector
	if injector, ok := opaqueCarrier.(propagation.Injector); ok {
		return injector(model.SpanContext(sc))
	}
	// fallback to support native opentracing http carrier
	if httpCarrier, ok := opaqueCarrier.(opentracing.HTTPHeadersCarrier); ok {
		req := &http.Request{Header: http.Header(httpCarrier)}
		switch p.tracer.opts.b3InjectOpt {
		case B3InjectSingle:
			return b3.InjectHTTP(req, b3.WithSingleHeaderOnly())(model.SpanContext(sc))
		case B3InjectBoth:
			return b3.InjectHTTP(req, b3.WithSingleAndMultiHeader())(model.SpanContext(sc))
		default:
			return b3.InjectHTTP(req)(model.SpanContext(sc))
		}
	}

	return opentracing.ErrInvalidCarrier
}

func (p *textMapPropagator) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {
	if extractor, ok := opaqueCarrier.(propagation.Extractor); ok {
		sc, err := extractor()
		if sc != nil {
			return SpanContext(*sc), err
		}
		return SpanContext{}, err
	}
	if httpCarrier, ok := opaqueCarrier.(opentracing.HTTPHeadersCarrier); ok {
		req := &http.Request{Header: http.Header(httpCarrier)}
		sc, err := b3.ExtractHTTP(req)()
		if sc != nil {
			return SpanContext(*sc), err
		}
		return SpanContext{}, err
	}
	return nil, opentracing.ErrUnsupportedFormat
}

type accessorPropagator struct {
	tracer *tracerImpl
}

func (p *accessorPropagator) Inject(
	spanContext opentracing.SpanContext,
	opaqueCarrier interface{},
) error {
	dc, ok := opaqueCarrier.(DelegatingCarrier)
	if !ok || dc == nil {
		return opentracing.ErrInvalidCarrier
	}
	sc, ok := spanContext.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	return dc.SetState(model.SpanContext(sc))
}

func (p *accessorPropagator) Extract(
	opaqueCarrier interface{},
) (opentracing.SpanContext, error) {
	dc, ok := opaqueCarrier.(DelegatingCarrier)
	if !ok || dc == nil {
		return nil, opentracing.ErrInvalidCarrier
	}

	sc, err := dc.State()
	return SpanContext(sc), err
}
