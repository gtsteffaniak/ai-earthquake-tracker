# AI Earthquake Tracker

This program scans certain publication websites for earthquake news and keeps track of the earthquakes based on AI interpretation of each article.

## How it works

1. Periodically scrapes articles on the web which post news about earthquakes via web crawler.
2. Sends new articles found to an LLM like chat-gpt for processing into a json presentation for processing.
3. Receives ai generated json with certain specific information:
    - Location (ie city name)
    - Date and Time
    - Magnitude
    - Deaths associated
    - Injuries associated
4. Updates table with information and marks article "read" so it will be ignored by future web crawling.
5. Displays the information on a web page for easy consumption.

## What it looks like:

### backend

details here

### UI

details here

