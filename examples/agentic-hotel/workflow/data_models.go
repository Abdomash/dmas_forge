package workflow

type Point struct {
	Pid  string  `bson:"pid"`
	Plat float64 `bson:"plat"`
	Plon float64 `bson:"plon"`
}

func (p Point) Id() string   { return p.Pid }
func (p Point) Lat() float64 { return p.Plat }
func (p Point) Lon() float64 { return p.Plon }

type RoomType struct {
	BookableRate       float64 `json:"bookable_rate" bson:"bookablerate"`
	Code               string  `json:"code" bson:"code"`
	RoomDescription    string  `json:"room_description" bson:"roomdescription"`
	TotalRate          float64 `json:"total_rate" bson:"totalrate"`
	TotalRateInclusive float64 `json:"total_rate_inclusive" bson:"totalrateinclusive"`
}

type RatePlan struct {
	HotelID string   `json:"hotel_id" bson:"hotelid"`
	Code    string   `json:"code" bson:"code"`
	RType   RoomType `json:"room_type" bson:"rtype"`
}

type Reservation struct {
	HotelID      string `json:"hotel_id" bson:"hotelid"`
	CustomerName string `json:"customer_name" bson:"customername"`
	InDate       string `json:"in_date" bson:"indate"`
	OutDate      string `json:"out_date" bson:"outdate"`
	Number       int64  `json:"number" bson:"number"`
}

type HotelNumber struct {
	HotelID string `json:"hotel_id" bson:"hotelid"`
	Number  int64  `json:"number" bson:"number"`
}

type Address struct {
	StreetNumber string  `json:"street_number" bson:"streetnumber"`
	StreetName   string  `json:"street_name" bson:"streetname"`
	City         string  `json:"city" bson:"city"`
	State        string  `json:"state" bson:"state"`
	Country      string  `json:"country" bson:"country"`
	PostalCode   string  `json:"postal_code" bson:"postalcode"`
	Lat          float64 `json:"lat" bson:"lat"`
	Lon          float64 `json:"lon" bson:"lon"`
}

type HotelProfile struct {
	ID          string  `json:"id" bson:"id"`
	Name        string  `json:"name" bson:"name"`
	PhoneNumber string  `json:"phone_number" bson:"phonenumber"`
	Description string  `json:"description" bson:"description"`
	Address     Address `json:"address" bson:"address"`
}

type Booking struct {
	BookingID    string `json:"booking_id" bson:"bookingid"`
	HotelID      string `json:"hotel_id" bson:"hotelid"`
	CustomerName string `json:"customer_name" bson:"customername"`
	InDate       string `json:"in_date" bson:"indate"`
	OutDate      string `json:"out_date" bson:"outdate"`
	RoomCount    int64  `json:"room_count" bson:"roomcount"`
	Status       string `json:"status" bson:"status"`
}
