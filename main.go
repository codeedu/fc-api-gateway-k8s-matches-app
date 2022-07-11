package main

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var log *zerolog.Logger

var tracer = otel.Tracer("echo-server")

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	logger := zerolog.New(output).With().Timestamp().Caller().Logger()
	log = &logger
}

func initTracer() (*sdktrace.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(os.Getenv("JAEGER_API"))))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		// Always be sure to batch in production.
		sdktrace.WithBatcher(exp),
		// Record information about this application in a Resource.
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("match"),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func main() {
	e := echo.New()
	// Middleware
	e.Logger.SetOutput(ioutil.Discard)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			req := c.Request()
			res := c.Response()
			start := time.Now()
			log.Debug().
				Interface("headers", req.Header).
				Msg(">>> " + req.Method + " " + req.RequestURI)
			if err = next(c); err != nil {
				c.Error(err)
			}
			log.Debug().
				Str("latency", time.Now().Sub(start).String()).
				Int("status", res.Status).
				Interface("headers", res.Header()).
				Msg("<<< " + req.Method + " " + req.RequestURI)
			return
		}
	})
	e.Use(middleware.Recover())
	//CORS
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.HEAD, echo.PUT, echo.PATCH, echo.POST, echo.DELETE},
	}))

	e.Static("/static", "assets/api-docs")

	tp, err := initTracer()
	if err != nil {
		log.Panic()
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	e.Use(otelecho.Middleware("match"))
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		ctx := c.Request().Context()
		oteltrace.SpanFromContext(ctx).RecordError(err)
		e.DefaultHTTPErrorHandler(err, c)
	}

	// Server
	e.GET("/api/matches/:id", GetMatch)
	e.GET("/health", Health)
	e.Logger.Fatal(e.Start(":9999"))
}

func Health(c echo.Context) error {
	return c.JSON(200, &HealthData{Status: "UP"})
}

type HealthData struct {
	Status string `json:"status,omitempty"`
}

func GetMatch(c echo.Context) error {
	m := &Match{
		HomeTeam:     "Barcelona",
		AwayTeam:     "Real Madrid",
		Championship: "UEFA",
	}
	return c.JSON(http.StatusOK, m)
}

type Match struct {
	HomeTeam     string `json:"homeTeam,omitempty"`
	AwayTeam     string `json:"awayTeam,omitempty"`
	Championship string `json:"championship,omitempty"`
}
