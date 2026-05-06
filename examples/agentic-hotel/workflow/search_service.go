package workflow

import "context"

type SearchServiceImpl struct {
	geoService  GeoService
	rateService RateService
}

func NewSearchServiceImpl(ctx context.Context, geoService GeoService, rateService RateService) (SearchService, error) {
	return &SearchServiceImpl{geoService: geoService, rateService: rateService}, nil
}

func (s *SearchServiceImpl) Nearby(ctx context.Context, lat float64, lon float64) ([]string, error) {
	ids, err := s.geoService.Nearby(ctx, lat, lon)
	if err != nil {
		return nil, err
	}

	rates, err := s.rateService.GetRates(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := []string{}
	for _, r := range rates {
		out = append(out, r.HotelID)
	}

	return out, nil
}
