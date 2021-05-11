package utils 

import (
    "encoding/json"
    "fmt"
    "github.com/PuerkitoBio/goquery"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "sync"
    "time"
)

type Asset struct {
    Symbol           string  `json:"symbol"`
    Name             string  `json:"name"`
    LastPrice        string  `json:"last_price"`
    PriceVariation   string  `json:"price_variation"`
}

type Quote struct {
    Date    string  `json:"date"`
    Price   string  `json:"price"`
}

type EODTicksObject struct {
    Content         EODTicks       `json:"d"`
}

type EODTicks struct {
    Name         string       `json:"Name"`
    Symbol       string       `json:"SymbolId"`
    Period       int          `json:"Xperiod"`
    ThreeDaysAgo Tick         `json:"qv"`
	CurrentDay   Tick         `json:"qd"`
	QuoteTab     []Tick       `json:"QuoteTab"`
}

type Tick struct {
    Date          int   	    `json:"d"`
    OpenPrice     float32   	`json:"o"`
    HighestPrice  float32   	`json:"h"`
    LowestPrice   float32   	`json:"l"`
	ClosingPrice  float32   	`json:"c"`
    Volume        uint   	    `json:"v"`
}

const (
    LayoutISO = "02/01/2006"
)
var DefaultDurations = []string{"1M","2M","3M","4M","5M","6M","7M","8M","9M","10M","11M","1Y","2Y","3Y"}
var DefaultPeriods = []string{"1","7","30","365"}

func ScrapeSearchResult(query string) ([]Asset, error) {
    sanitizedQuery := url.QueryEscape(strings.TrimSpace(query))
    doc, err := getHTMLDocument("http://www.boursorama.com/recherche/ajax?query=" + sanitizedQuery)
    if err != nil {
        return nil, err
    }

    // Find the search results
    var assets []Asset
    doc.Find(".search__list").First().Find(".search__list-link").Each(func(i int, s *goquery.Selection) {
        asset := Asset{}

        otherInfo := strings.Trim(s.Find(".search__item-content").Text(), " \n")
        name := s.Find(".search__item-title").Text()
        asset.Name = name + "\n" + otherInfo

        link, ok := s.Attr("href")
        if !ok {
            log.Fatalf("Unable to find the quote symbol for %s\n", asset.Name)
            return
        }
        var symbolIndex int
        splittedLink := strings.Split(link, "/")
        runeLink := []rune(link)
        if (runeLink[len(runeLink) - 1] == []rune("/")[0]) {
          symbolIndex = len(splittedLink) - 2
        } else {
          symbolIndex = len(splittedLink) - 1
        }
        asset.Symbol = splittedLink[symbolIndex]

        searchInstrument := s.Find(".search__item-instrument")

        asset.LastPrice = searchInstrument.Find(".last").Text()

        asset.PriceVariation = searchInstrument.Find("[class^=u-color]").Text()

        assets = append(assets, asset)
    })

    return assets, nil
}

func GetQuotes(symbol string, startDate time.Time, duration string, period string) ([]Quote, error) {
    if ok := contains(DefaultDurations, duration); !ok {
        return nil, fmt.Errorf("Duration must be one of %v", DefaultDurations)
    }
    if ok := contains(DefaultPeriods, period); !ok {
        return nil, fmt.Errorf("Period must be one of %v", DefaultPeriods)
    }

    // First page request to get the number of pages to scrap
    url := getQuotesUrl(symbol, startDate, duration, period, 1)
    doc, err := getHTMLDocument(url)
    if err != nil {
        return nil, err
    }

    nbOfPages := doc.Find("span.c-pagination__content").Length()

    scrapQuotes := func() ([]Quote) {
        quotes := []Quote{}
        doc.Find(".c-table tr").Each(func(i int, s *goquery.Selection) {
            // Escape first row (table header)
            if i == 0 {
                return
            }
            firstCell := s.Find(".c-table__cell").First()
            quote := Quote{}
            quote.Date = strings.TrimSpace(firstCell.Text())
            quote.Price = strings.TrimSpace(firstCell.Next().Text())
            quotes = append(quotes, quote)
        })
        return quotes
    }

    var allQuotes []Quote

    // Fetch quotes concurrently if there is more than one page
    if (nbOfPages < 2) {
        allQuotes = scrapQuotes()
    } else {
        // Make channels to pass fatal errors in WaitGroup
        fatalErrors := make(chan error)
        wgDone := make(chan bool)

        var wg sync.WaitGroup
        // Scrap by page
        getPageQuotes := func(index int) ([]Quote, error) {
            url = getQuotesUrl(symbol, startDate, duration, period, index + 1)
            doc, err = getHTMLDocument(url)
            if err != nil {
                return nil, err
            }
            return scrapQuotes(), nil
        }
        // Init slice to return quotes from all pages
        quotesByPage := make([][]Quote, nbOfPages)
        // Use first page request to scrap quotes
        quotesByPage[0] = scrapQuotes()
        // Fetch the remaining pages
        for i := 1; i < nbOfPages; i++ {
            wg.Add(1)

            go func(index int) {
                defer wg.Done()

                quotesByPage[index], err = getPageQuotes(index)
                if err != nil {
                    fatalErrors <- err
                }
            }(i)
        }

        // Final goroutine to wait until WaitGroup is done
        go func() {
            wg.Wait()
            close(wgDone)
        }()

        // Wait until either WaitGroup is done or an error is received through the channel
        select {
        case <-wgDone:
            // Carry on
            break
        case err := <-fatalErrors:
            close(fatalErrors)
            return nil, err
        }

        for _, currentPageQuotes := range(quotesByPage) {
            allQuotes = append(allQuotes, currentPageQuotes...)
        }
    }

    return allQuotes, nil
}

func getHTMLDocument(url string) (*goquery.Document, error) {
    // Request the HTML page
	res, err := http.Get(url)
    if err != nil {
        return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
        return nil, err
	}
    return doc, nil
}

func getQuotesUrl(symbol string, startDate time.Time, duration string, period string, page int) string {
    sanitizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
    if page == 1 {
        return "http://www.boursorama.com/_formulaire-periode/?symbol=" + sanitizedSymbol + "&historic_search[startDate]=" + startDate.Format(LayoutISO) + "&historic_search[duration]=" + duration + "&historic_search[period]=" + period
    } else {
        return "http://www.boursorama.com/_formulaire-periode/page-" + strconv.Itoa(page) + "?symbol=" + sanitizedSymbol + "&historic_search[startDate]=" + startDate.Format(LayoutISO) + "&historic_search[duration]=" + duration + "&historic_search[period]=" + period
    }
}

func GetEODTicks(symbol string, days string) (*EODTicksObject, error) {
	sanitizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	length := strings.ToUpper(strings.TrimSpace(days))

	url := "https://www.boursorama.com/bourse/action/graph/ws/GetTicksEOD?symbol=" + sanitizedSymbol + "&length=" + length + "&period=0&guid="
    res, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()
    if res.StatusCode != 200 {
        return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
    }
    var ticks EODTicksObject

    body, err := ioutil.ReadAll(res.Body)
    if err != nil {
        return nil, err
    }


    err = json.Unmarshal(body, &ticks)
    if err != nil {
        return nil, err
    }

    for idx := range ticks.Content.QuoteTab {
        var timestamp, err = parseTimestamp(ticks.Content.QuoteTab[idx].Date)
        if err != nil {
            fmt.Errorf("Couldn't parse timestamp", err)
        }
        ticks.Content.QuoteTab[idx].Date = int(timestamp)
    }

    return &ticks, nil
}

func UpdateEODTicks(symbol string) string {
    sanitizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
    return "https://www.boursorama.com/bourse/action/graph/ws/UpdateCharts?symbol=" + sanitizedSymbol + "&period=-1"
}

func parseTimestamp(minuteTimestamp int) (int64, error) {
    s := strconv.Itoa(minuteTimestamp)
    year, err := strconv.Atoi(s[0:2])
    if err != nil {
        return 0, err
    }
    year += 2000
    month, err := strconv.Atoi(s[2:4])
    if err != nil {
        return 0, err
    }
    day, err := strconv.Atoi(s[4:6])
    if err != nil {
        return 0, err
    }
    min, err := strconv.Atoi(s[6:10])
    if err != nil {
        return 0, err
    }

    hours := min/60
    minutes := min - (hours*60)
    return time.Date(year, time.Month(month), day, hours, minutes, 0, 0, time.Local).Unix()*1000, nil
}

func contains(values []string, query string) bool {
    for _, value := range values {
        if value == query {
            return true
        }
    }
    return false
}
