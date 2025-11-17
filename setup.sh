#!/bin/bash

# Rely OpenSearch Setup Script
# This script helps you get started with the OpenSearch storage backend

set -e

echo "🚀 Rely + OpenSearch Setup"
echo "=========================="
echo ""

# Check prerequisites
echo "📋 Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.24+ first."
    exit 1
fi

echo "✅ All prerequisites met"
echo ""

# Create .env if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file from template..."
    cp .env.example .env
    echo "✅ .env file created"
    echo "   You can edit .env to customize your configuration"
else
    echo "ℹ️  .env file already exists, skipping..."
fi
echo ""

# Start OpenSearch
echo "🐳 Starting OpenSearch with Docker Compose..."
docker-compose up -d

echo ""
echo "⏳ Waiting for OpenSearch to be ready..."
timeout 60 bash -c 'until curl -s http://localhost:9200/_cluster/health > /dev/null 2>&1; do sleep 2; done' || {
    echo "❌ OpenSearch failed to start within 60 seconds"
    echo "   Check logs with: docker-compose logs opensearch"
    exit 1
}

echo "✅ OpenSearch is ready!"
echo ""

# Download dependencies
echo "📦 Downloading Go dependencies..."
go mod download
go mod tidy
echo "✅ Dependencies ready"
echo ""

# Success message
echo "✨ Setup complete!"
echo ""
echo "📍 Services:"
echo "   OpenSearch:   http://localhost:9200"
echo "   Dashboards:   http://localhost:5601"
echo ""
echo "🎯 Next steps:"
echo "   1. Run the example:"
echo "      go run examples/opensearch/main.go"
echo ""
echo "   2. Or use Make:"
echo "      make example-opensearch"
echo ""
echo "   3. Connect a Nostr client to:"
echo "      ws://localhost:3334"
echo ""
echo "   4. View stored events in OpenSearch Dashboards:"
echo "      http://localhost:5601"
echo ""
echo "📖 Documentation:"
echo "   Quick Start:  cat QUICKSTART.md"
echo "   Full Guide:   cat OPENSEARCH_SETUP.md"
echo ""
echo "Happy relaying! ⚡"
