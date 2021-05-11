package api

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/jeanpic/stock-alert-backend/internal/utils"
)

func RegisterHandlers(router *gin.Engine) {

    v1 := router.Group("/api/v1")

    v1.GET("/search", func(c *gin.Context) {
        q := c.Query("q")
        if q == "" {
            handleBadRequest(c, "Missing query value")
            return
        }

        results, err := utils.ScrapeSearchResult(q);
        if err != nil {
            handleBadRequest(c, err)
            return
        }
        c.JSON(http.StatusOK, results)
    })

    v1.GET("/quotes/:symbol", func(c *gin.Context) {
        symbol := c.Param("symbol")

        now := time.Now()
        lastMonth := now.AddDate(0,-1,0)
        // Default start date = a month from now
        startDate := c.DefaultQuery("startDate", lastMonth.Format(utils.LayoutISO))
        startDateAsTime, err := time.Parse(utils.LayoutISO, startDate)
        if err != nil {
            handleBadRequest(c, err)
            return
        }
        // Default duration = 3 months
        duration := c.DefaultQuery("duration", "3M")
        // Default period = daily
        period := c.DefaultQuery("period", "1")

        quotes, err := utils.GetQuotes(symbol, startDateAsTime, duration, period)
        if err != nil {
            handleBadRequest(c, err)
            return
        }

        c.JSON(http.StatusOK, quotes)
    })

    v1.GET("/ticks/:symbol", func(c *gin.Context) {
        symbol := c.Param("symbol")
		days := c.DefaultQuery("days", "1")

        ticks, err := utils.GetEODTicks(symbol, days)
        if err != nil {
            handleBadRequest(c, err)
            return
        }

        c.JSON(http.StatusOK, ticks)
    })
}

func handleBadRequest(c *gin.Context, message interface{}) {
    c.JSON(http.StatusBadRequest, gin.H{
        "status": http.StatusBadRequest,
        "message": message,
    })
}
