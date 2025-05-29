package etf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ananthakumaran/paisa/internal/config"
	"github.com/ananthakumaran/paisa/internal/model/etf"
	"github.com/ananthakumaran/paisa/internal/model/mutualfund/scheme"
	"github.com/ananthakumaran/paisa/internal/model/portfolio"
	"github.com/ananthakumaran/paisa/internal/model/price"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type JustEtfPriceProvider struct {
}

func (p *JustEtfPriceProvider) Code() string {
	return "justetf"
}

func (p *JustEtfPriceProvider) Label() string {
	return "Just ETF"
}

func (p *JustEtfPriceProvider) Description() string {
	return "ETF Support for EU providers."
}

func (p *JustEtfPriceProvider) AutoCompleteFields() []price.AutoCompleteField {
	return []price.AutoCompleteField{
		{Label: "ISIN", ID: "isin", Help: "ISIN of ETF", InputType: "text"},
	}
}

func (p *JustEtfPriceProvider) AutoComplete(db *gorm.DB, field string, filter map[string]string) []price.AutoCompleteItem {
	// count := scheme.Count(db)
	// if count == 0 {
	// 	schemes, err := GetSchemes()
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	scheme.UpsertAll(db, schemes)
	// } else {
	// 	log.Info("Using cached results")
	// }

	switch field {
	case "amc":
		return scheme.GetAMCCompletions(db)
	case "scheme":
		return scheme.GetNAVNameCompletions(db, filter["amc"])
	}
	return []price.AutoCompleteItem{}
}

func (p *JustEtfPriceProvider) ClearCache(db *gorm.DB) {
	db.Exec("DELETE FROM schemes")
}

func (p *JustEtfPriceProvider) GetPrices(code string, commodityName string) ([]*price.Price, error) {

	var prices []*price.Price
	etfPrices, err := getETFPricing(code, config.DefaultCurrency())
	if err != nil {
		return prices, err
	}
	for _, etfPrice := range etfPrices.Series {
		date, err := time.ParseInLocation("2006-01-02", etfPrice.Date, config.TimeZone())
		log.Println(date, etfPrice.Date)
		if err != nil {
			return prices, err
		}
		price := price.Price{Date: date, CommodityType: config.ETF, CommodityID: code, CommodityName: commodityName, Value: decimal.NewFromFloat(etfPrice.Value.Raw)}
		prices = append(prices, &price)
	}
	log.Println(len(prices))
	return prices, nil
}

func GetJustETFPortfolio(db *gorm.DB, isin string, commodityName string) ([]*portfolio.Portfolio, error) {
	var stds []Standard

	var etfInformation *etf.ETFInformation
	err := db.First(&etfInformation, "isin=?", isin).Error
	if etfInformation == nil || err == gorm.ErrRecordNotFound {
		etf, err := getETF(isin)
		log.Println(etf.PartnerType, err)
		if err != nil {
			return nil, err
		}
		etfInformation = etf
		db.Save(etf)
	}
	if etfInformation.PartnerType == "Xtrackers" {
		stds = ParseXtrackers(etfInformation.RawData)
	} else if etfInformation.PartnerType == "Amundi ETF" {
		stds = ParseAmundi(etfInformation.RawData)
	}

	var portfolios []*portfolio.Portfolio
	for _, data := range stds {
		if data.Name == "ADYEN NV" {
			log.Println(data.ISIN, etfInformation.PartnerType)
		}
		portfolio := portfolio.Portfolio{
			SecurityName:      data.Name,
			CommodityType:     config.ETF,
			SecurityID:        data.ISIN,
			Percentage:        decimal.NewFromFloat(data.Weighting),
			ParentCommodityID: isin,
			SecurityRating:    data.Rating,
			SecurityIndustry:  data.Industry,
			SecurityType:      data.SecurityType,
			SecurityCountry:   data.Country}
		portfolios = append(portfolios, &portfolio)
	}
	log.Println(len(portfolios))
	return portfolios, nil
}

func getETF(isin string) (*etf.ETFInformation, error) {
	url := fmt.Sprintf("https://www.justetf.com/en/etf-profile.html?isin=%s", isin)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	respStr := string(respBytes)
	partner := strings.Split(strings.Split(respStr, `jtrck-partner="`)[1], `"`)[0]
	etf := &etf.ETFInformation{ISIN: isin, PartnerType: partner}
	if partner == "Xtrackers" {
		url := fmt.Sprintf("https://etf.dws.com/etfdata/export/GBR/ENG/excel/product/constituent/%s/", isin)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		etf.RawData = respBytes
		return etf, nil
	} else if partner == "Amundi ETF" {
		data := fmt.Sprintf(`{"context":{"countryCode":"NLD","countryName":"Netherlands","googleCountryCode":"NL","domainName":"www.amundietf.nl","bcp47Code":"en-GB","languageName":"English","gtmCode":"GTM-KR9BS5J","languageCode":"en","userProfileName":"RETAIL","userProfileSlug":"retail","portalProfileName":null,"portalProfileSlug":null},"productIds":["%s"],"productType":"PRODUCT","composition":{"compositionFields":["date","type","bbg","isin","name","weight","quantity","currency","sector","country","countryOfRisk"]}}`, isin)

		req, err := http.NewRequest("POST", "https://www.amundietf.nl/mapi/ProductAPI/getProductsData", strings.NewReader(data))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:136.0) Gecko/20100101 Firefox/136.0")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", "https://www.amundietf.nl/")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://www.amundietf.nl")
		req.Header.Set("DNT", "1")
		req.Header.Set("Sec-GPC", "1")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Cookie", "countryCode=NLD; userProfileName=RETAIL; languageName=English; version-consent-cookie=3; consent-cookie_marketing=explicitely_granted; consent-cookie_analytics=explicitely_granted; consent-cookie_functionality=explicitely_granted")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		etf.RawData = respBytes
		return etf, nil
	} else {
		log.Panicln("unknown partner", partner)
		return nil, nil
	}
}

func getETFPricing(isin string, currency string) (PriceHistory, error) {
	var priceHistory PriceHistory
	today := time.Now()
	threeMonthsbefore := today.Add(-time.Hour * 24 * 30)
	layout := "2006-01-02"
	url := fmt.Sprintf("https://www.justetf.com/api/etfs/%s/performance-chart?locale=en&currency=%s&valuesType=MARKET_VALUE&reduceData=true&includeDividends=false&dateFrom=%s&dateTo=%s", isin, currency, threeMonthsbefore.Format(layout), today.Format(layout))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return priceHistory, err
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return priceHistory, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return priceHistory, err
	}
	err = json.Unmarshal(respBytes, &priceHistory)
	if err != nil {
		return priceHistory, err
	}
	return priceHistory, nil

}

type PriceHistory struct {
	Series []struct {
		Date  string `json:"date"`
		Value struct {
			Raw       float64 `json:"raw"`
			Localized string  `json:"localized"`
		} `json:"value"`
	} `json:"series"`
}
