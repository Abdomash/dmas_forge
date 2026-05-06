package workflow

import "context"

type FrontendServiceImpl struct {
	search      SearchService
	profile     ProfileService
	reservation ReservationService
	advisor     HotelAdvisorAgent
	support     SupportAgent
}

func NewFrontendServiceImpl(ctx context.Context, search SearchService, profile ProfileService, reservation ReservationService, advisor HotelAdvisorAgent, support SupportAgent) (FrontendService, error) {
	return &FrontendServiceImpl{
		search:      search,
		profile:     profile,
		reservation: reservation,
		advisor:     advisor,
		support:     support,
	}, nil
}

func (f *FrontendServiceImpl) SearchHotels(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if msg := validateStay(req.InDate, req.OutDate, req.RoomCount); msg != "" {
		return SearchResult{Status: "invalid_request", Message: msg}, nil
	}

	locale := req.Locale
	if locale == "" {
		locale = "en"
	}

	ids, err := f.search.Nearby(ctx, req.Lat, req.Lon)
	if err != nil {
		return SearchResult{}, err
	}

	ids, err = f.reservation.CheckAvailability(ctx, req.CustomerName, ids, req.InDate, req.OutDate, req.RoomCount)
	if err != nil {
		return SearchResult{}, err
	}

	hotels, err := f.profile.GetProfiles(ctx, ids, locale)
	if err != nil {
		return SearchResult{}, err
	}

	return SearchResult{Status: "ok", Hotels: hotels}, nil
}

func (f *FrontendServiceImpl) BookRoom(ctx context.Context, req BookingRequest) (BookingResult, error) {
	if req.HotelID == "" {
		return BookingResult{Status: "invalid_request", Message: "hotel_id is required"}, nil
	}

	if msg := validateStay(req.InDate, req.OutDate, req.RoomCount); msg != "" {
		return BookingResult{Status: "invalid_request", Message: msg}, nil
	}

	return f.reservation.MakeReservation(ctx, req.CustomerName, []string{req.HotelID}, req.InDate, req.OutDate, req.RoomCount)
}

func (f *FrontendServiceImpl) PlanStay(ctx context.Context, req AdvisorRequest) (AdvisorResult, error) {
	return f.advisor.PlanStay(ctx, req)
}

func (f *FrontendServiceImpl) AskSupport(ctx context.Context, req SupportRequest) (SupportResult, error) {
	return f.support.AskSupport(ctx, req)
}

func validateStay(inDate, outDate string, roomCount int64) string {
	if !validDate(inDate) || !validDate(outDate) {
		return "dates must use YYYY-MM-DD"
	}

	if roomCount <= 0 {
		return "room_count must be positive"
	}

	if inDate >= outDate {
		return "out_date must be after in_date"
	}

	return ""
}

func validDate(s string) bool { return len(s) == 10 && s[4] == '-' && s[7] == '-' }
