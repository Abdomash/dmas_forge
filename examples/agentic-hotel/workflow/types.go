package workflow

import "context"

type FrontendService interface {
	SearchHotels(ctx context.Context, req SearchRequest) (SearchResult, error)
	BookRoom(ctx context.Context, req BookingRequest) (BookingResult, error)
	PlanStay(ctx context.Context, req AdvisorRequest) (AdvisorResult, error)
	AskSupport(ctx context.Context, req SupportRequest) (SupportResult, error)
}

type GeoService interface {
	Nearby(ctx context.Context, lat float64, lon float64) ([]string, error)
}

type RateService interface {
	GetRates(ctx context.Context, hotelIDs []string) ([]RatePlan, error)
}

type ProfileService interface {
	GetProfiles(ctx context.Context, hotelIDs []string, locale string) ([]HotelProfile, error)
}

type SearchService interface {
	Nearby(ctx context.Context, lat float64, lon float64) ([]string, error)
}

type ReservationService interface {
	CheckAvailability(ctx context.Context, customerName string, hotelIDs []string, inDate string, outDate string, roomCount int64) ([]string, error)
	MakeReservation(ctx context.Context, customerName string, hotelIDs []string, inDate string, outDate string, roomCount int64) (BookingResult, error)
	GetBooking(ctx context.Context, bookingID string) (Booking, error)
}

type HotelAdvisorAgent interface {
	PlanStay(ctx context.Context, req AdvisorRequest) (AdvisorResult, error)
}

type SupportAgent interface {
	AskSupport(ctx context.Context, req SupportRequest) (SupportResult, error)
}

type SearchRequest struct {
	CustomerName string  `json:"customer_name"`
	InDate       string  `json:"in_date"`
	OutDate      string  `json:"out_date"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	RoomCount    int64   `json:"room_count"`
	Locale       string  `json:"locale"`
}

type SearchResult struct {
	Status  string         `json:"status"`
	Message string         `json:"message,omitempty"`
	Hotels  []HotelProfile `json:"hotels"`
}

type BookingRequest struct {
	InDate       string `json:"in_date"`
	OutDate      string `json:"out_date"`
	HotelID      string `json:"hotel_id"`
	CustomerName string `json:"customer_name"`
	RoomCount    int64  `json:"room_count"`
}

type BookingResult struct {
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	BookingID string `json:"booking_id,omitempty"`
	HotelID   string `json:"hotel_id,omitempty"`
}

type AdvisorRequest struct {
	Prompt string `json:"prompt"`
}

type AdvisorResult struct {
	Answer string `json:"answer"`
}

type SupportRequest struct {
	Question string `json:"question"`
}

type SupportResult struct {
	Answer string `json:"answer"`
}
