package results

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/librespeed/speedtest/config"
	"github.com/librespeed/speedtest/database"
	"github.com/librespeed/speedtest/database/schema"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/image/font"
)

const (
	watermark = "LibreSpeed"

	labelMS       = " ms"
	labelMbps     = "Mbit/s"
	labelPing     = "Ping"
	labelJitter   = "Jitter"
	labelDownload = "Download"
	labelUpload   = "Upload"
)

var (
	ipv4Regex     = regexp.MustCompile(`(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)
	ipv6Regex     = regexp.MustCompile(`(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4})?:)?((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9]))`)
	hostnameRegex = regexp.MustCompile(`"hostname":"([^\\\\"]|\\\\")*"`)

	fontLight, fontBold                                                                                                *truetype.Font
	pingJitterLabelFace, upDownLabelFace, pingJitterValueFace, upDownValueFace, smallLabelFace, ispFace, watermarkFace font.Face

	canvasWidth, canvasHeight = 500, 286
	dpi                       = 150.0
	topOffset                 = 10
	middleOffset              = topOffset + 5
	bottomOffset              = middleOffset - 10
	ispOffset                 = bottomOffset + 8
	colorLabel                = image.NewUniform(color.RGBA{40, 40, 40, 255})
	colorDownload             = image.NewUniform(color.RGBA{96, 96, 170, 255})
	colorUpload               = image.NewUniform(color.RGBA{96, 96, 96, 255})
	colorPing                 = image.NewUniform(color.RGBA{170, 96, 96, 255})
	colorJitter               = image.NewUniform(color.RGBA{170, 96, 96, 255})
	colorMeasure              = image.NewUniform(color.RGBA{40, 40, 40, 255})
	colorISP                  = image.NewUniform(color.RGBA{40, 40, 40, 255})
	colorWatermark            = image.NewUniform(color.RGBA{160, 160, 160, 255})
	colorSeparator            = image.NewUniform(color.RGBA{192, 192, 192, 255})
)

type Result struct {
	ProcessedString string         `json:"processedString"`
	RawISPInfo      IPInfoResponse `json:"rawIspInfo"`
}

type IPInfoResponse struct {
	IP           string `json:"ip"`
	Hostname     string `json:"hostname"`
	City         string `json:"city"`
	Region       string `json:"region"`
	Country      string `json:"country"`
	Location     string `json:"loc"`
	Organization string `json:"org"`
	Postal       string `json:"postal"`
	Timezone     string `json:"timezone"`
	Readme       string `json:"readme"`
}

func Initialize(c *config.Config) {
	// changed to use Noto Sans instead of OpenSans, due to issue:
	// https://github.com/golang/freetype/issues/8
	if b, err := ioutil.ReadFile(filepath.Join(c.AssetsPath, "NotoSansDisplay-Light.ttf")); err != nil {
		log.Fatalf("Error opening NotoSansDisplay-Light font: %s", err)
	} else {
		f, err := freetype.ParseFont(b)
		if err != nil {
			log.Fatalf("Error parsing NotoSansDisplay-Light font: %s", err)
		}
		fontLight = f
	}

	if b, err := ioutil.ReadFile(filepath.Join(c.AssetsPath, "NotoSansDisplay-Medium.ttf")); err != nil {
		log.Fatalf("Error opening NotoSansDisplay-Medium font: %s", err)
	} else {
		f, err := freetype.ParseFont(b)
		if err != nil {
			log.Fatalf("Error parsing NotoSansDisplay-Medium font: %s", err)
		}
		fontBold = f
	}

	pingJitterLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    12,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	upDownLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    14,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	pingJitterValueFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    16,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	upDownValueFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    18,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	smallLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    10,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	ispFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    8,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	watermarkFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    6,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
}

func Record(w http.ResponseWriter, r *http.Request) {
	ipAddr, _, _ := net.SplitHostPort(r.RemoteAddr)
	userAgent := r.UserAgent()
	language := r.Header.Get("Accept-Language")

	ispInfo := r.FormValue("ispinfo")
	download := r.FormValue("dl")
	upload := r.FormValue("ul")
	ping := r.FormValue("ping")
	jitter := r.FormValue("jitter")
	logs := r.FormValue("log")
	extra := r.FormValue("extra")

	if config.LoadedConfig().RedactIP {
		ipAddr = "0.0.0.0"
		ipv4Regex.ReplaceAllString(ispInfo, "0.0.0.0")
		ipv4Regex.ReplaceAllString(logs, "0.0.0.0")
		ipv6Regex.ReplaceAllString(ispInfo, "0.0.0.0")
		ipv6Regex.ReplaceAllString(logs, "0.0.0.0")
		hostnameRegex.ReplaceAllString(ispInfo, `"hostname":"REDACTED"`)
		hostnameRegex.ReplaceAllString(logs, `"hostname":"REDACTED"`)
	}

	var record schema.TelemetryData
	record.IPAddress = ipAddr
	if ispInfo == "" {
		record.ISPInfo = "{}"
	} else {
		record.ISPInfo = ispInfo
	}
	record.Extra = extra
	record.UserAgent = userAgent
	record.Language = language
	record.Download = download
	record.Upload = upload
	record.Ping = ping
	record.Jitter = jitter
	record.Log = logs

	t := time.Now()
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0)
	uuid := ulid.MustNew(ulid.Timestamp(t), entropy)
	record.UUID = uuid.String()

	err := database.DB.Insert(&record)
	if err != nil {
		log.Errorf("Error inserting into database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := w.Write([]byte("id " + uuid.String())); err != nil {
		log.Errorf("Error writing ID to telemetry request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func DrawPNG(w http.ResponseWriter, r *http.Request) {
	uuid := r.FormValue("id")
	record, err := database.DB.FetchByUUID(uuid)
	if err != nil {
		log.Errorf("Error querying database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var result Result
	if err := json.Unmarshal([]byte(record.ISPInfo), &result); err != nil {
		log.Errorf("Error parsing ISP info: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	canvas := image.NewRGBA(image.Rectangle{
		Min: image.Point{},
		Max: image.Point{
			X: canvasWidth,
			Y: canvasHeight,
		},
	})

	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	drawer := &font.Drawer{
		Dst:  canvas,
		Face: pingJitterLabelFace,
	}

	drawer.Src = colorLabel

	// labels
	p := drawer.MeasureString(labelPing)
	x := canvasWidth/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight/10+topOffset)
	drawer.DrawString(labelPing)

	p = drawer.MeasureString(labelJitter)
	x = canvasWidth*3/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight/10+topOffset)
	drawer.DrawString(labelJitter)

	drawer.Face = upDownLabelFace
	p = drawer.MeasureString(labelDownload)
	x = canvasWidth/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight/2-middleOffset)
	drawer.DrawString(labelDownload)

	p = drawer.MeasureString(labelUpload)
	x = canvasWidth*3/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight/2-middleOffset)
	drawer.DrawString(labelUpload)

	drawer.Face = smallLabelFace
	drawer.Src = colorMeasure
	p = drawer.MeasureString(labelMbps)
	x = canvasWidth/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight*8/10-middleOffset)
	drawer.DrawString(labelMbps)

	p = drawer.MeasureString(labelMbps)
	x = canvasWidth*3/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight*8/10-middleOffset)
	drawer.DrawString(labelMbps)

	msLength := drawer.MeasureString(labelMS)

	// ping value
	drawer.Face = pingJitterValueFace
	pingValue := strings.Split(record.Ping, ".")[0]
	p = drawer.MeasureString(pingValue)

	x = canvasWidth/4 - (p.Round()+msLength.Round())/2
	drawer.Dot = freetype.Pt(x, canvasHeight*11/40)
	drawer.Src = colorPing
	drawer.DrawString(pingValue)
	x = x + p.Round()
	drawer.Dot = freetype.Pt(x, canvasHeight*11/40)
	drawer.Src = colorMeasure
	drawer.Face = smallLabelFace
	drawer.DrawString(labelMS)

	// jitter value
	drawer.Face = pingJitterValueFace
	p = drawer.MeasureString(record.Jitter)
	x = canvasWidth*3/4 - (p.Round()+msLength.Round())/2
	drawer.Dot = freetype.Pt(x, canvasHeight*11/40)
	drawer.Src = colorJitter
	drawer.DrawString(record.Jitter)
	drawer.Face = smallLabelFace
	x = x + p.Round()
	drawer.Dot = freetype.Pt(x, canvasHeight*11/40)
	drawer.Src = colorMeasure
	drawer.DrawString(labelMS)

	// download value
	drawer.Face = upDownValueFace
	p = drawer.MeasureString(record.Download)
	x = canvasWidth/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight*27/40-middleOffset)
	drawer.Src = colorDownload
	drawer.DrawString(record.Download)

	// upload value
	p = drawer.MeasureString(record.Upload)
	x = canvasWidth*3/4 - p.Round()/2
	drawer.Dot = freetype.Pt(x, canvasHeight*27/40-middleOffset)
	drawer.Src = colorUpload
	drawer.DrawString(record.Upload)

	// watermark
	ctx := freetype.NewContext()
	ctx.SetFont(fontLight)
	ctx.SetFontSize(14)
	ctx.SetDPI(dpi)
	ctx.SetHinting(font.HintingFull)

	drawer.Face = watermarkFace
	drawer.Src = colorWatermark
	p = drawer.MeasureString(watermark)
	x = canvasWidth - p.Round() - 5
	drawer.Dot = freetype.Pt(x, canvasHeight-bottomOffset)
	drawer.DrawString(watermark)

	// timestamp
	ts := record.Timestamp.Format("2006-01-02 15:04:05")
	p = drawer.MeasureString(ts)
	drawer.Dot = freetype.Pt(8, canvasHeight-bottomOffset)
	drawer.DrawString(ts)

	// separator
	for i := canvas.Bounds().Min.X; i < canvas.Bounds().Max.X; i++ {
		canvas.Set(i, canvasHeight-ctx.PointToFixed(6).Round()-bottomOffset, colorSeparator)
	}

	// ISP info
	drawer.Face = ispFace
	drawer.Src = colorISP
	drawer.Dot = freetype.Pt(8, canvasHeight-ctx.PointToFixed(6).Round()-ispOffset)
	var ispString string
	if strings.Contains(result.ProcessedString, "-") {
		str := strings.SplitN(result.ProcessedString, "-", 2)
		if strings.Contains(str[1], "(") {
			str = strings.SplitN(str[1], "(", 2)
		}
		ispString = str[0]
	}
	drawer.DrawString("ISP: " + ispString)

	w.Header().Set("Content-Disposition", "inline; filename="+uuid+".png")
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, canvas); err != nil {
		log.Errorf("Failed to output image to HTTP client: %s", err)
	}
}
