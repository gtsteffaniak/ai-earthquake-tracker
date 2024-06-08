package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/google/generative-ai-go/genai"
	"github.com/gtsteffaniak/html-web-crawler/crawler"
	"golang.org/x/net/html"
	"google.golang.org/api/option"
)

const (
	tableName = "ai-earthquake-tracker"
	region    = "us-east-1"
)

type Item struct {
	ID          string  `json:"id"`
	LastUpdated string  `json:"lastUpdated"`
	Injured     int     `json:"injured"`
	Deaths      int     `json:"deaths"`
	Magnitude   float64 `json:"magnitude"`
	Location    string  `json:"location"`
	Date        string  `json:"date"`
	RefUrl      string  `json:"refUrl"`
}

var (
	visited   = map[string]bool{}
	ctx       = context.Background()
	model     *genai.GenerativeModel
	tableData = []Item{}
)

func main() {
	_ = setupDBClient() // dynamodb client
	tableInfo, err := getTableContents(tableName)
	if err != nil {
		log.Fatal(err)
	}

	err = attributevalue.UnmarshalListOfMaps(tableInfo.Items, &tableData)
	if err != nil {
		log.Fatalf("failed to unmarshal items, %v", err)
	}
	go setupWeb()
	model = setupLLMClient()        // ai client
	Crawler := crawler.NewCrawler() // web crawler
	// Add crawling HTML selector classes
	Crawler.Selectors.Classes = []string{"PageList-items-item", "Topics"}
	Crawler.Selectors.Ids = []string{"root"}
	Crawler.Selectors.LinkTextPatterns = []string{"quake"}
	Crawler.Selectors.UrlPatterns = []string{"quake"}
	// Allow 5 consecutive pages to crawl at a time
	Crawler.Threads = 1
	for _, item := range tableData {
		visited[item.RefUrl] = true
		Crawler.IgnoredUrls = append(Crawler.IgnoredUrls, item.RefUrl)
	}

	for {
		crawledData, _ := Crawler.Crawl(
			"https://apnews.com/hub/earthquakes",
			"https://www.aljazeera.com/tag/earthquakes/",
			"https://abcnews.go.com/alerts/earthquakes",
		)
		fmt.Println("Total: ", len(crawledData))
		for url, data := range crawledData {
			if visited[url] {
				continue
			}
			visited[url] = true
			if len(data) > 1000000 {
				fmt.Printf("Skipping %v \n lines: %v \n", url, len(data))
				continue
			}
			sanitizedText, err := sanitizeHTML(data)
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Printf("lines: %v \n", len(sanitizedText)/100)
			// Call Gemini API with sanitizedText
			r, err := getLLMResponse(model, sanitizedText)
			if err != nil {
				log.Println(err)
				continue
			}
			processResponse(r, url)
			// https://ai.google.dev/pricing
			// rate limit to 4 requests per minute
			time.Sleep(15 * time.Second)
		}
		time.Sleep(24 * time.Hour)
	}
}

// Function to process the JSON response
func processResponse(r string, refUrl string) {
	var info Item

	err := json.Unmarshal([]byte(r), &info)
	if err != nil {
		log.Printf("Failed to unmarshal response: %v", err)
		fmt.Println(r)
		return
	}

	// create ID based on magnitude and location and date
	hasher := md5.New()
	YYYYMM := info.Date[:len(info.Date)-3]
	hasher.Write([]byte(fmt.Sprintf("%v-%v-%v", info.Magnitude, info.Location, YYYYMM)))
	hash := hasher.Sum(nil)
	info.ID = hex.EncodeToString(hash)
	info.LastUpdated = time.Now().Format("2006-01-02")
	info.RefUrl = refUrl

	b, err := json.Marshal(info)
	if err != nil {
		log.Printf("Failed to marshal item: %v", err)
		return
	}
	if info.Magnitude == 0 || info.Date == "unknown" || info.Location == "unknown" {
		// discard the item if any of the required fields are missing
		return
	}
	err = UpdateOrInsertItem(tableName, info)
	if err != nil {
		log.Printf("Failed to update or insert item: %v", err)
	}
	fmt.Println(string(b))
}

func setupLLMClient() *genai.GenerativeModel {
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	model := client.GenerativeModel("gemini-1.5-flash") // change model?
	return model
}

// This function is not includedin the provided code, but represents the logic for calling the Gemini API
func getLLMResponse(model *genai.GenerativeModel, text string) (string, error) {
	prompt := `
	output a json with the following format:
	{"deaths": 0, "injured": 0, "magnitude": 4.7, "location": "City, State", "date": "YYYY-MM-DD"}
	(int) deaths/injured value should come from term like "[number] people died/dead/killed/injured" or in words like "killing at least three" as deaths or "injuring ten" as injured value. 0 if int is missing.
	(string) location must be in the format "City, State" or "City, Country" if outside the US. "unknown" if not confident in location.
	(string) date must be in the format "YYYY-MM-DD", "unknown" if date is missing
	` + text
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}
	responseString := ""
	// Show the model's response, which is expected to be text.
	for _, part := range resp.Candidates[0].Content.Parts {
		responseString += fmt.Sprintf("%v", part)
	}
	responseString = strings.Trim(responseString, "`")
	responseString = strings.Replace(responseString, "`", "", -1)
	responseString = strings.Trim(responseString, "json")
	responseString = strings.Trim(responseString, "\n")
	// Return an empty string if no response is found
	return responseString, err
}

// sanitizeText removes non-alphanumeric and non-punctuation characters from a string.
func sanitizeText(text string) string {
	var sanitized strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSpace(r) {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

// removeStyleAndScript removes <style> and <script> elements from the HTML node.
func removeStyleAndScript(n *html.Node) {
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		if c.Type == html.ElementNode && (c.Data == "style" || c.Data == "script") {
			n.RemoveChild(c)
		} else {
			removeStyleAndScript(c)
		}
	}
}

// extractBodyText recursively extracts and sanitizes text content from an HTML node.
func extractBodyText(n *html.Node) string {
	if n.Type == html.TextNode {
		return sanitizeText(n.Data)
	}
	var result strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result.WriteString(extractBodyText(c))
	}
	return result.String()
}

// findBodyNode finds the body element in the parsed HTML document.
func findBodyNode(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if body := findBodyNode(c); body != nil {
			return body
		}
	}
	return nil
}

// sanitizeSpaces removes empty lines and reduces multiple spaces to a single space.
func sanitizeSpaces(text string) string {
	// Remove empty lines
	lines := strings.Split(text, "\n")
	var nonEmptyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 {
			nonEmptyLines = append(nonEmptyLines, trimmed)
		}
	}
	text = strings.Join(nonEmptyLines, " ")
	// Reduce multiple spaces to a single space
	re := regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	return text
}

// sanitizeHTML sanitizes the input HTML string and returns the sanitized text from the body element.
func sanitizeHTML(htmlStr string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", err
	}
	body := findBodyNode(doc)
	if body == nil {
		return "", fmt.Errorf("no body element found")
	}
	removeStyleAndScript(body)
	text := extractBodyText(body)
	return sanitizeSpaces(text), nil
}
