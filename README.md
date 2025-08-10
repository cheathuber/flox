# flox

[![Go Report Card](https://goreportcard.com/badge/github.com/cheathuber/flox)](https://goreportcard.com/report/github.com/cheathuber/flox)
[![Build Status](https://github.com/cheathuber/flox/actions/workflows/build-deb.yml/badge.svg)](https://github.com/cheathuber/flox/actions/workflows/build-deb.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![GitHub issues](https://img.shields.io/github/issues/cheathuber/flox)](https://github.com/cheathuber/flox/issues)
[![GitHub stars](https://img.shields.io/github/stars/cheathuber/flox)](https://github.com/cheathuber/flox/stargazers)


**flox** is the project powering [flox.click](https://flox.click) - a simple, user-friendly platform that lets anyone easily create and deploy small static websites under personalized subdomains. The goal is to empower users to spin up their own sites quickly with seamless DNS configuration and intuitive site management.

## 🚀 Vision

- **Easy site creation:** Validate and register unique site names quickly.
- **Automated DNS management:** Integrated with deSEC DNS API to provision DNS A records automatically.
- **Configurable and extensible:** Backend built in Go with modular architecture for future CMS and content enhancements.
- **Lightweight and scalable:** Using a filesystem-based backend for storage simplicity in early stages.
- **Secure and stable:** Planned improvements for reservation locking, session management, and authentication.

## 🧩 Project Components

- **Backend (`backend/`):**  
  Go HTTP server handling site validation, creation, configuration storage, and DNS integration.

- **Frontend (planned):**  
  A user interface to guide users through site name selection, validation feedback, site creation, and content editing.

- **DNS integration:**  
  Uses the deSEC API for dynamic DNS record management to ensure new sites become reachable immediately.

## 📦 Current Status

- Prototype backend running with site validation and creation endpoints.
- Stores per-site config files under configurable directories.
- A-record creation automated via environment-configured deSEC API token and IP.
- Configuration managed via `.env` for easy local and production setups.
- Reservation locking and advanced session handling planned.

## ⚡ Getting Started

Please see [backend/README.md](backend/README.md) for detailed backend setup, running instructions, and API usage.

## 🎯 Roadmap

- Frontend UI and user-friendly workflows.
- Reservation locking to prevent race conditions in site creation.
- CMS installation /  features for content editing and management.
- Authentication and user sessions.
- Support for additional DNS record types and advanced features.

---

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

*Enjoy building and deploying with flox.click!*  
Feel free to open issues and contribute! 🚀

---

> _This project is under active development._  
> _Contributions and suggestions are welcome!_
