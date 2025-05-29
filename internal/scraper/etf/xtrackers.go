package etf

import (
	"log"
	"strings"

	"github.com/szyhf/go-excel"
)

type Standard struct {
	ID           int
	Name         string  `xlsx:"column(Name)"`
	ISIN         string  `xlsx:"column(ISIN)"`
	Country      string  `xlsx:"column(Country)"`
	SecurityType string  `xlsx:"column(Type of Security)"`
	Industry     string  `xlsx:"column(Industry Classification)"`
	Weighting    float64 `xlsx:"column(Weighting)"`
	Rating       string  `xlsx:"column(Rating)"`
}

func ParseXtrackers(rawInformation []byte) []Standard {
	conn := excel.NewConnecter()
	err := conn.OpenBinary(rawInformation)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	sheet := conn.GetSheetNames()[0]
	rd, err := conn.NewReaderByConfig(&excel.Config{TitleRowIndex: 1, Sheet: sheet})
	if err != nil {
		panic(err)
	}
	defer rd.Close()

	securities := make([]Standard, 0)
	for rd.Next() {
		var s Standard
		// Read a row into a struct.
		err := rd.Read(&s)
		if err != nil {
			log.Println(err)
			continue
		}
		if s.Rating == "-" {
			s.Rating = ""
		}
		s.SecurityType = strings.ToLower(s.SecurityType)
		log.Println(s.Weighting)
		securities = append(securities, s)
	}
	return securities
}
