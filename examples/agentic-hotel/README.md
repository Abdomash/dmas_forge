# Agentic Hotel

Agentic Hotel is a small hotel reservation example inspired by [DeathStarBench Hotel Reservation](https://github.com/delimitrou/DeathStarBench/tree/master/hotelReservation). It keeps the same general shape as the DSB service graph and adapts some of its search, reservation, and seeded-data ideas, then adds two agent-facing services behind the same frontend API.

The goal is to show how a conventional microservice workflow can sit next to agent services without making every service agentic. The regular services handle hotel search, profiles, rates, availability, and booking. The agents handle natural-language pre-booking advice and post-booking support.

This is an example/benchmark-style app. It is useful for wiring, deployment, and service-interaction experiments, not as production hotel software.

## Architecture

```text
FrontendService
│
├── /SearchHotels
│   ├── SearchService
│   │   ├── GeoService
│   │   │   └── [owns: geo MongoDB]
│   │   │
│   │   └── RateService
│   │       └── [owns: rate MongoDB]
│   │
│   ├── ReservationService
│   │   └── [owns: reservation MongoDB]
│   │
│   └── ProfileService
│       └── [owns: profile MongoDB]
│
├── /BookRoom
│   └── ReservationService
│
├── /PlanStay
│   └── HotelAdvisorAgent
│       ├── search_hotels tool -> SearchService
│       ├── get_rates tool -> RateService
│       ├── get_profiles tool -> ProfileService
│       └── check_availability tool -> ReservationService
│
└── /AskSupport
    └── SupportAgent [RAG-embedded support docs]
        └── lookup_booking tool -> ReservationService
```

Bracketed leaves are owned data sources or embedded knowledge, not deployable workflow services. Tool leaves are agent-callable operations, not public frontend routes.

In the default `single` wiring, only `FrontendService` is exposed over HTTP. The other services are internal dependencies. Other wirings can deploy each service separately over HTTP, A2A, or MCP.

## Services

`FrontendService` is the public API for the example. It exposes four operations: `SearchHotels`, `BookRoom`, `PlanStay`, and `AskSupport`.

`GeoService` owns hotel coordinates. It loads seeded San Francisco-area hotel points and returns up to five nearby hotel IDs within a 10 km radius.

`RateService` owns hotel rate plans. It returns the seeded rate plans for a list of hotel IDs. Rates are keyed by hotel ID only in this example.

`ProfileService` owns user-facing hotel details. It returns names, descriptions, phone numbers, and addresses for hotel IDs.

`SearchService` combines `GeoService` and `RateService`. It asks `GeoService` for nearby hotel IDs, then asks `RateService` for rates and returns the hotel IDs that have rate plans.

`ReservationService` owns availability and booking state. It checks whether hotels have enough rooms for a date range, creates confirmed bookings, and looks up existing bookings by booking ID.

`HotelAdvisorAgent` is a pre-booking assistant. It can search hotels, read rates and profiles, and check availability through read-only tools. It cannot create bookings or look up existing bookings.

`SupportAgent` is a post-booking and policy assistant. It uses automatic read-only RAG over embedded markdown support docs and can call `lookup_booking` when the user provides a booking ID. It cannot search hotels, check availability, create bookings, change bookings, or cancel bookings.

## Frontend API

The generated HTTP API uses `GET` requests with a URL-encoded `req` query parameter containing JSON.

Replace `12345` with the frontend port from your generated deployment.

### SearchHotels

Searches near a coordinate, filters by availability, and returns hotel profiles.

Request fields:

| Field | Type | Notes |
| --- | --- | --- |
| `customer_name` | string | Optional for search; passed to availability logic. |
| `in_date` | string | Required. Format: `YYYY-MM-DD`. |
| `out_date` | string | Required. Must be after `in_date`. |
| `lat` | number | Search latitude. |
| `lon` | number | Search longitude. |
| `room_count` | integer | Required. Must be positive. |
| `locale` | string | Optional. Defaults to `en`. |

Response shape:

```json
{
  "status": "ok",
  "hotels": [
    {
      "id": "1",
      "name": "Hotel name",
      "phone_number": "...",
      "description": "...",
      "address": {
        "street_number": "...",
        "street_name": "...",
        "city": "San Francisco",
        "state": "CA",
        "country": "United States",
        "postal_code": "...",
        "lat": 37.78,
        "lon": -122.41
      }
    }
  ]
}
```

Example:

```bash
curl --get 'http://localhost:12345/SearchHotels' \
  --data-urlencode 'req={"in_date":"2015-04-09","out_date":"2015-04-10","lat":37.7835,"lon":-122.41,"room_count":1,"locale":"en"}'
```

### BookRoom

Checks availability for one hotel and creates a booking if rooms are available.

Request fields:

| Field | Type | Notes |
| --- | --- | --- |
| `in_date` | string | Required. Format: `YYYY-MM-DD`. |
| `out_date` | string | Required. Must be after `in_date`. |
| `hotel_id` | string | Required. |
| `customer_name` | string | Booking customer name. |
| `room_count` | integer | Required. Must be positive. |

Response shape:

```json
{
  "status": "confirmed",
  "booking_id": "...",
  "hotel_id": "1"
}
```

If the hotel has no capacity, the status is `unavailable`. If the request is invalid, the status is `invalid_request` with a message.

Example:

```bash
curl --get 'http://localhost:12345/BookRoom' \
  --data-urlencode 'req={"in_date":"2015-04-09","out_date":"2015-04-10","hotel_id":"1","customer_name":"Bob","room_count":1}'
```

### PlanStay

Asks `HotelAdvisorAgent` for natural-language planning help before booking. The agent can use read-only hotel search, rate, profile, and availability tools.

Request fields:

| Field | Type | Notes |
| --- | --- | --- |
| `prompt` | string | User request for hotel discovery or stay planning. |

Response shape:

```json
{
  "answer": "..."
}
```

Example:

```bash
curl --get 'http://localhost:12345/PlanStay' \
  --data-urlencode 'req={"prompt":"Find a San Francisco hotel near Union Square for 2015-04-09 to 2015-04-10 for one room."}'
```

### AskSupport

Asks `SupportAgent` for booking or policy support. The agent automatically retrieves relevant embedded support docs and can look up a booking if the question includes a booking ID.

Request fields:

| Field | Type | Notes |
| --- | --- | --- |
| `question` | string | Support or policy question. |

Response shape:

```json
{
  "answer": "..."
}
```

Example:

```bash
curl --get 'http://localhost:12345/AskSupport' \
  --data-urlencode 'req={"question":"What is the check-in policy?"}'
```

## Data And Storage

Deployment uses one MongoDB container per stateful service: geo, rate, profile, and reservation. There are no caches.

Seed data is reset at startup. The example generates 80 hotels over an approximate San Francisco grid so geo search has distinct city-scale coverage without hand-maintaining real hotel addresses.

The support knowledge base is embedded from markdown files in `workflow/data/support`. Current docs cover booking changes, cancellation, check-in, and payment.

Reservations are stored as simple booking records plus per-day room reservations. Booking IDs are generated when a reservation is confirmed.

## Model Config

`wiring/example_model.json` configures both the chat model and `embedding_model`:

```json
{
  "name": "gpt-5.4-nano",
  "url": "https://api.openai.com/v1",
  "key": "your-api-key-here",
  "embedding_model": "text-embedding-3-small"
}
```

`HotelAdvisorAgent` uses the chat model for tool-using planning. `SupportAgent` uses the chat model plus the embedding model because it indexes support docs into the RAG knowledge base at startup.

## Build And Run

```bash
cd examples/agentic-hotel/wiring
go run main.go -w single -o build -modfile=./example_model.json
cd build/docker
cp ../.local.env .env
docker compose build && docker compose up -d
```

Available wiring modes:

| Mode | What it deploys |
| --- | --- |
| `single` | One frontend HTTP endpoint with internal services in the same process/container. |
| `http` | Each workflow service as its own HTTP service. |
| `a2a` | Internal services over A2A, frontend exposed over HTTP. |
| `mcp` | Internal services over MCP, frontend exposed over HTTP. |

## Notes And Limitations

This example intentionally keeps the domain model small. Rates are keyed by hotel ID only; dates affect reservation availability and booking records, not rate lookup.

MongoDB avoids in-process data races, but reservation booking semantics intentionally match simple DSB-style benchmark logic and are not transactionally serializable.

The agents are scoped by design. `HotelAdvisorAgent` is for discovery before booking. `SupportAgent` is for policy and booking lookup after booking. Booking creation remains a normal frontend operation through `BookRoom`.
