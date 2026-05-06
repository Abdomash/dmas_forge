package workflow

import "fmt"

const (
	seedHotelCount  = 80
	seedGridColumns = 10
	seedGridRows    = (seedHotelCount + seedGridColumns - 1) / seedGridColumns

	// Approximate San Francisco bounding box. The generator spreads hotels over
	// this grid to provide distinct city-scale geo coverage without maintaining
	// 80 hand-curated real hotel locations.
	seedMinLat = 37.72
	seedMaxLat = 37.81
	seedMinLon = -122.50
	seedMaxLon = -122.38

	seedRateCode                = "RACK"
	seedBaseRate                = 109.0
	seedRateStep                = 25.0
	seedRateBuckets             = 6
	seedInclusiveRateMultiplier = 1.13

	seedBaseCapacity    = int64(120)
	seedCapacityStep    = int64(30)
	seedCapacityBuckets = 5
)

var seedNeighborhoods = []string{
	"Union Square",
	"SoMa",
	"Embarcadero",
	"Mission",
	"Nob Hill",
	"Marina",
	"Castro",
	"Richmond",
	"Sunset",
	"Presidio",
}

var seedHotelStyles = []string{
	"Hotel",
	"Inn",
	"Suites",
	"Lodge",
	"House",
	"Resort",
	"Apartments",
	"Court",
}

var seedStreetNames = []string{
	"Market St",
	"Mission St",
	"Geary Blvd",
	"Van Ness Ave",
	"Lombard St",
	"California St",
	"Howard St",
	"Folsom St",
	"Pine St",
	"Post St",
}

var seedRoomTypes = []RoomType{
	{Code: "KNG", RoomDescription: "King sized bed"},
	{Code: "QN", RoomDescription: "Queen sized bed"},
	{Code: "DBL", RoomDescription: "Two double beds"},
}

type seedHotel struct {
	id           string
	name         string
	phone        string
	description  string
	streetNumber string
	streetName   string
	postalCode   string
	lat          float64
	lon          float64
	roomType     RoomType
	rate         float64
	capacity     int64
}

func seedHotels() []seedHotel {
	hotels := make([]seedHotel, 0, seedHotelCount)
	latStep := (seedMaxLat - seedMinLat) / float64(seedGridRows-1)
	lonStep := (seedMaxLon - seedMinLon) / float64(seedGridColumns-1)

	for i := 1; i <= seedHotelCount; i++ {
		idx := i - 1
		row := idx / seedGridColumns
		col := idx % seedGridColumns
		neighborhood := seedNeighborhoods[col%len(seedNeighborhoods)]
		style := seedHotelStyles[row%len(seedHotelStyles)]
		rate := seedBaseRate + float64(idx%seedRateBuckets)*seedRateStep
		roomType := seedRoomTypes[idx%len(seedRoomTypes)]
		roomType.BookableRate = rate
		roomType.TotalRate = rate
		roomType.TotalRateInclusive = rate * seedInclusiveRateMultiplier

		hotels = append(hotels, seedHotel{
			id:           fmt.Sprintf("%d", i),
			name:         fmt.Sprintf("%s %s %02d", neighborhood, style, i),
			phone:        fmt.Sprintf("(415) 555-%04d", 1000+i),
			description:  fmt.Sprintf("A comfortable %s-area stay with easy access to San Francisco dining, transit, and attractions.", neighborhood),
			streetNumber: fmt.Sprintf("%d", 100+i*7),
			streetName:   seedStreetNames[idx%len(seedStreetNames)],
			postalCode:   fmt.Sprintf("941%02d", 1+idx%10),
			lat:          seedMinLat + float64(row)*latStep,
			lon:          seedMinLon + float64(col)*lonStep,
			roomType:     roomType,
			rate:         rate,
			capacity:     seedBaseCapacity + int64(idx%seedCapacityBuckets)*seedCapacityStep,
		})
	}

	return hotels
}

func seedPoints() []Point {
	points := make([]Point, 0, seedHotelCount)
	for _, h := range seedHotels() {
		points = append(points, Point{Pid: h.id, Plat: h.lat, Plon: h.lon})
	}
	return points
}

func seedRates() []RatePlan {
	rates := make([]RatePlan, 0, seedHotelCount)
	for _, h := range seedHotels() {
		rates = append(rates, RatePlan{HotelID: h.id, Code: seedRateCode, RType: h.roomType})
	}
	return rates
}

func seedProfiles() []HotelProfile {
	profiles := make([]HotelProfile, 0, seedHotelCount)
	for _, h := range seedHotels() {
		profiles = append(profiles, HotelProfile{
			ID:          h.id,
			Name:        h.name,
			PhoneNumber: h.phone,
			Description: h.description,
			Address: Address{
				StreetNumber: h.streetNumber,
				StreetName:   h.streetName,
				City:         "San Francisco",
				State:        "CA",
				Country:      "United States",
				PostalCode:   h.postalCode,
				Lat:          h.lat,
				Lon:          h.lon,
			},
		})
	}
	return profiles
}

func seedHotelNumbers() []HotelNumber {
	numbers := make([]HotelNumber, 0, seedHotelCount)
	for _, h := range seedHotels() {
		numbers = append(numbers, HotelNumber{HotelID: h.id, Number: h.capacity})
	}
	return numbers
}
