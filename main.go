package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	resource2 "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type TestOutput struct {
	Time    string `json:"Time"`
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func parseTestOutput(output string) (*TestOutput, error) {
	var testOutput TestOutput
	err := json.Unmarshal([]byte(output), &testOutput)
	if testOutput.Action != "output" {
		return nil, fmt.Errorf("ignoring line %s", testOutput.Output)
	}
	if !strings.Contains(testOutput.Output, "PASS") && !strings.Contains(testOutput.Output, "FAIL") {
		return nil, fmt.Errorf("ignoring line %s", testOutput.Output)
	}
	return &testOutput, err
}

func CreateTrace(to *TestOutput) {
	ctx := context.Background()
	tracer := otel.Tracer("gotestracer")

	ctx, span := tracer.Start(ctx, to.Test)
	defer span.End()

	span.SetAttributes(
		attribute.String("package", to.Package),
		attribute.String("action", to.Action),
		attribute.String("output", to.Output),
	)
}

type FileSpanExporter struct {
	writer io.Writer
}

func (f *FileSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		spanData := map[string]interface{}{
			"traceID":    span.SpanContext().TraceID().String(),
			"spanID":     span.SpanContext().SpanID().String(),
			"name":       span.Name(),
			"attributes": span.Attributes(),
			"events":     span.Events(),
			"links":      span.Links(),
			"status":     span.Status(),
		}
		jsonData, err := json.Marshal(spanData)
		if err != nil {
			return err
		}
		if _, err := f.writer.Write(jsonData); err != nil {
			return err
		}
		if _, err := f.writer.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

func (f *FileSpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

func initTracer() {
	exporter := &FileSpanExporter{writer: os.Stdout}

	resource := resource2.NewWithAttributes("TestTracesService")
	batchSpan := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithBatchTimeout(time.Second),
	)

	tracer := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource),
		sdktrace.WithSpanProcessor(batchSpan),
	)
	otel.SetTracerProvider(tracer)
	propagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagator)
}

func main() {
	initTracer()
	reader := bufio.NewReader(os.Stdin)

	for {
		testLine, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("Error reading: %+v", err)
		}
		testOutput, err := parseTestOutput(testLine)
		if err != nil {
			continue
		}
		CreateTrace(testOutput)
	}

	time.Sleep(3000 * time.Millisecond)
}
