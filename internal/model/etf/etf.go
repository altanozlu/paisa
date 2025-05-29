package etf

type ETFInformation struct {
	ISIN        string `gorm:"primaryKey"`
	PartnerType string
	RawData     []byte
}
