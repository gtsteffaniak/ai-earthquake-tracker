package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/generative-ai-go/genai"
	"github.com/gtsteffaniak/html-web-crawler/crawler"
	"golang.org/x/net/html"
	"google.golang.org/api/option"
)

var (
	visited = map[string]bool{}
	ctx     = context.Background()
)

func main() {
	// Set up the client and model
	model := setupClient()

	Crawler := crawler.NewCrawler()
	// Add crawling HTML selector classes
	Crawler.Selectors.Classes = []string{"PageList-items-item"}
	// Allow 50 consecutive pages to crawl at a time
	Crawler.Threads = 50
	for {
		fmt.Print("looping")
		// Crawl starting with a given URL
		crawledData, _ := Crawler.Crawl("https://apnews.com/hub/earthquakes")
		fmt.Println("Total: ", len(crawledData))
		for url, data := range crawledData {
			if visited[url] {
				continue
			}
			visited[url] = true
			if len(data) > 1000000 {
				fmt.Printf("Skipping %v \n size: %v \n", url, len(data))
				continue
			}
			sanitizedText, err := sanitizeHTML(data)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			// Call Gemini API with sanitizedText
			r, err := getLLMResponse(model, sanitizedText)
			if err != nil {
				fmt.Println("Error calling Gemini API:", err)
				continue
			}

			// Process the Gemini response (e.g., print explanation)
			fmt.Println("Gemini explanation:", r)

		}
		time.Sleep(1 * time.Minute)
	}
}

func setupClient() *genai.GenerativeModel {
	// Access your API key as an environment variable (see "Set up your API key" above)
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("API_KEY")))
	if err != nil {
		log.Fatal(err)
	}

	// The Gemini 1.5 models are versatile and work with most use cases
	model := client.GenerativeModel("gemini-1.5-flash")
	return model
}

// This function is not includedin the provided code, but represents the logic for calling the Gemini API
func getLLMResponse(model *genai.GenerativeModel, text string) (string, error) {
	prompt := `
	output a json object with the information , with the following format:
	{"deaths": "0", "injured": "0", "magnitude": "4.7", "location": "Falls City, Texas", "date": "2024-06-05"}
	keep string and integers consistent and return empty string for missing string values and 0 for empty integer values
	` + text
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Fatal(err)
	}
	responseString := ""
	// Show the model's response, which is expected to be text.
	for _, part := range resp.Candidates[0].Content.Parts {
		responseString += fmt.Sprintf("%v", part)
	}
	responseString = strings.Trim(responseString, "`")
	responseString = strings.Trim(responseString, "json")
	responseString = strings.Trim(responseString, "\n")
	// Return an empty string if no response is found
	return responseString, nil
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
