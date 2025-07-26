package etf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/ananthakumaran/paisa/internal/config"
	gql "github.com/ananthakumaran/paisa/internal/gql"
	"github.com/ananthakumaran/paisa/internal/model/mutualfund/scheme"
	"github.com/ananthakumaran/paisa/internal/model/portfolio"
	"github.com/ananthakumaran/paisa/internal/model/price"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type BeursPriceProvider struct {
}

func (p *BeursPriceProvider) Code() string {
	return "beurs"
}

func (p *BeursPriceProvider) Label() string {
	return "Beurs"
}

func (p *BeursPriceProvider) Description() string {
	return "ETF Support for EU providers."
}

func (p *BeursPriceProvider) AutoCompleteFields() []price.AutoCompleteField {
	return []price.AutoCompleteField{
		{Label: "ISIN", ID: "isin", Help: "ISIN of ETF", InputType: "text"},
	}
}

func (p *BeursPriceProvider) AutoComplete(db *gorm.DB, field string, filter map[string]string) []price.AutoCompleteItem {
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

func (p *BeursPriceProvider) ClearCache(db *gorm.DB) {
	db.Exec("DELETE FROM schemes")
}

func (p *BeursPriceProvider) GetPrices(code string, commodityName string) ([]*price.Price, error) {

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

func GetBeursPortfolio(db *gorm.DB, isin string, commodityName string) ([]*portfolio.Portfolio, error) {
	var portfolios []*portfolio.Portfolio

	ctx := context.Background()
	client := graphql.NewClient("http://127.0.0.1:4040/", http.DefaultClient)
	resp, err := gql.Etf(ctx, client, gql.GetSymbolOptions{Currency: "EUR", Text: isin, PreferredExchange: "GETTEX"})
	if err != nil {
		return portfolios, err
	}

	for _, data := range resp.GetEtfInformation.GetHoldings() {
		portfolio := portfolio.Portfolio{
			SecurityName:      data.GetName(),
			CommodityType:     config.ETF,
			Percentage:        decimal.NewFromFloat(data.GetWeighting()),
			ParentCommodityID: isin,
			SecurityType:      strings.Replace(data.GetTypename(), "Etfholding", "", 1),
			SecurityCountry:   data.GetCountry()}
		if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingEquity); ok {
			pe := decimal.NewFromFloat(holding.GetStockInformation().PriceEarningsRatio)
			portfolio.SecurityID = holding.Isin
			portfolio.PriceEarningsRatio = pe
			portfolio.SecurityIndustry = holding.Industry
		} else if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingBond); ok {
			portfolio.SecurityID = holding.Isin
		} else if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingRight); ok {
			portfolio.SecurityID = holding.Isin
		} else if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingMutualFund); ok {
			portfolio.SecurityID = holding.Isin
		} else if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingFuture); ok {
			portfolio.SecurityID = holding.Isin
		} else if holding, ok := data.(*gql.EtfGetEtfInformationEtfHoldingsEtfholdingCash); ok {
			portfolio.SecurityID = holding.GetName()
		}
		if portfolio.Percentage.IsZero() {
			continue
		}
		portfolios = append(portfolios, &portfolio)
	}
	return portfolios, nil
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
