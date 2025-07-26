# flox Backend

This repository contains the backend service for [flox.click](https://flox.click), responsible for site name validation, site creation, and DNS record provisioning via the deSEC API.

## Features (Prototype Phase)

- Site name validation with syntax checks, blacklist, and existence check.
- Site creation API with atomic directory creation as "locking" mechanism.
- Stores site configuration (`config.json`) with metadata.
- Integrates with deSEC API to automatically create DNS A records.
- Configurable via environment variables (`.env`).

## Getting Started

### Prerequisites

- Go 1.18+ installed
- deSEC API token with write permissions
- Internet access to call deSEC API

### Setup

1. Clone the repo and enter backend directory:

```bash
git clone https://github.com/cheathuber/flox.git
cd flox/backend
```

2. Create a `.env` file at the project root (`../.env`) with the following variables:

```env
SITES_BASE_DIR=/var/www/flox/sites
SITE_IP=1.2.3.4
DNS_API_RRSETS=desec.io/api/v1/domains/flox.click/rrsets/
DNS_API_AUTH="Token your-desec-api-token"
```

3. Get dependencies:

```bash
go mod tidy
```

4. Run the backend:

```bash
go run main.go
```

### API Endpoints

- **POST /api/sites/validate-name**

  Validate a site name.

  **Request JSON:**

  ```json
  {
    "siteName": "example"
  }
  ```

  **Response JSON:**

  ```json
  {
    "valid": true,
    "error": "optional error message if invalid"
  }
  ```

- **POST /api/sites**

  Create a site with configuration and DNS record.

  **Request JSON:**

  ```json
  {
    "siteName": "example",
    "description": "My site",
    "style": "modern",
    "initialContent": ["blog", "contact"]
  }
  ```

  **Response JSON:**

  ```json
  {
    "success": true,
    "siteUrl": "https://example.flox.click",
    "error": "optional error message if creation failed"
  }
  ```

## Project Structure

- `main.go`: entrypoint with HTTP handlers and core logic.
- `sites/`: directory where all site folders and configs are stored (configurable).
- `.env`: environment variables for configuration (API tokens, directories, IPs).

## Future Enhancements

- Reservation locking system to avoid late-race conflicts.
- User session or reservation tokens for UX improvements.
- Site content provisioning and CMS integration.
- More DNS record types support and error handling improvements.
- Authentication and security.
