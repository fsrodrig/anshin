# Anshin: original brief

> **Status: starting point, not a spec.** Everything below is debatable. The
> owner's own summary of what they actually want:
>
> "Monitor my Linux PCs' resources (CPU, memory, disk, battery, temperature)
> easily from my Android phone, everything connected over Tailscale securely.
> Grab the best ideas from how apps like Monitee, Beszel and Netdata work."
>
> What was actually decided lives in [`design.md`](design.md), the decision
> records in [`adr/`](adr/) and the domain language in
> [`GLOSSARY.md`](../GLOSSARY.md). Where those disagree with this brief, they win.

---

# ANSHIN
## Private Linux Monitoring for Android

You are a senior software engineer specializing in Android, Go, Linux systems, networking, security, and product design.

Your mission is to design and implement **Anshin**, a beautiful, lightweight, privacy-first Android application for monitoring Linux computers over a private Tailscale network.

Build a complete, functional, maintainable, open-source MVP, not just a prototype or architecture proposal.

Make pragmatic technical decisions, prioritize simplicity, and avoid unnecessary infrastructure.

---

# 1. Product Vision

**Product name:** Anshin

**Tagline:** Your systems. At peace.

**Brand philosophy:**

Anshin (安心) is a Japanese word expressing peace of mind, reassurance, and a sense of security.

The product should embody this philosophy:

*Your machines are being watched over, so you can focus on everything else.*

Anshin combines three fundamental concepts:

1. **Omakase:** Thoughtful defaults, trust, delegation, and effortless operation.
2. **Pulse:** Real-time awareness of system health and activity.
3. **Node:** Private, interconnected Linux computers.

The application should feel like a small, reliable guardian for Linux machines.

It should be quiet, precise, dependable, and unobtrusive.

The user should be able to understand the health of all monitored machines within three seconds of opening the app.

## Primary use case

A user has:

- Two Linux computers.
- Both connected to the same Tailscale tailnet.
- An Android phone connected to that tailnet.
- No desire to expose monitoring services to the public Internet.
- A preference for lightweight, self-hosted software.
- A preference for minimalist, technical, beautifully designed interfaces.

The system must support more than two machines, but optimize the initial experience for two.

## Core principles

- Privacy by design.
- No subscriptions.
- No accounts.
- No cloud backend.
- No advertisements.
- No telemetry or tracking.
- No central monitoring server.
- No unnecessary background activity.
- Minimal CPU, RAM, network, and battery usage.
- Excellent Android user experience.
- Secure defaults.
- Easy installation and maintenance.

---

# 2. Brand Identity

## Brand name

**Anshin**

Preferred visual representation:

`anshin`

Use lowercase typography for the wordmark.

The brand should feel technical but approachable, minimalist but recognizable.

Avoid excessive Japanese cultural decoration or stereotypical visual elements.

Japanese influence should come from restraint, harmony, precision, and simplicity.

## Brand personality

Anshin should communicate:

- Calm confidence
- Reliability
- Technical precision
- Privacy
- Simplicity
- Thoughtful craftsmanship

Imagine a beautifully designed Linux utility that happens to live on Android.

## Logo concept

Design an original, geometric, minimalist symbol combining:

- A connected node
- A subtle heartbeat or pulse
- A sense of protection and stability

Explore an abstract icon formed by two connected points and a restrained pulse line.

The icon must:

- Work in monochrome.
- Remain recognizable at 24×24 pixels.
- Work as an Android adaptive icon.
- Support light and dark themes.
- Be visually balanced.
- Avoid unnecessary details.
- Avoid copying existing logos.

Deliver vector SVG source assets, Android adaptive icon resources, and a monochrome icon.

Use the wordmark `anshin` alongside the symbol where appropriate.

Do not use a heart-shaped logo or generic shield icon.

---

# 3. Design Direction — Omarchy Linux

The primary visual inspiration is **Omarchy Linux**, including its cohesive theme system, minimalism, terminal-inspired aesthetics, and carefully chosen color palettes.

References:

- https://omarchy.org/
- https://omarchy.org/manual/themes/

Study these references before implementing the visual system, if web access is available.

The objective is not to copy Omarchy's interface, but to translate its visual philosophy into a polished native Android experience.

Additional inspiration:

- btop
- Modern terminal user interfaces
- Hyprland environments
- Well-designed developer tools
- Minimalist system dashboards

## Design principles

1. Terminal-inspired, not a terminal emulator.
2. Dense enough to be useful, but never cluttered.
3. Precise typography and consistent spacing.
4. Subtle borders instead of excessive elevation.
5. Restrained colors with meaningful accents.
6. Minimal animations.
7. Clear information hierarchy.
8. High-quality data visualization.
9. Native Android interactions.
10. No unnecessary visual decoration.

Avoid:

- Generic enterprise dashboards.
- Oversized Material Design cards.
- Excessive corner rounding.
- Glassmorphism.
- Gradients.
- Decorative illustrations.
- Heavy shadows.
- Neon cyberpunk aesthetics.
- Large empty spaces that reduce information density.

## Typography

Use a combination of:

**Primary UI font**
- A clean, modern sans-serif.
- Prefer system typography when appropriate.

**Technical font**
- JetBrains Mono or another high-quality open-source monospaced font.
- Use for metrics, percentages, hostnames, IP addresses, timestamps, and technical labels.

Use tabular numerals for changing numerical values.

Typography must remain readable on small Android screens.

---

# 4. Theme System

Implement a centralized theme engine using semantic design tokens.

Initial themes:

1. Kanagawa — default
2. Tokyo Night
3. Catppuccin Mocha
4. Matte Black

Use authentic upstream color palettes where licensing permits.

Each theme should define:

- Background
- Surface
- Elevated surface
- Border
- Primary text
- Secondary text
- Muted text
- Accent
- CPU chart color
- Memory chart color
- Storage chart color
- Network chart color
- Success
- Warning
- Critical
- Offline

Do not hardcode colors directly inside UI components.

## Default theme: Kanagawa

The default appearance should feature:

- Deep charcoal backgrounds.
- Warm, muted foreground text.
- Soft blue CPU indicators.
- Muted purple memory indicators.
- Desaturated green storage indicators.
- Subtle borders.
- Calm, understated status colors.

The interface should feel comfortable during extended use, especially in dark environments.

Allow users to switch themes from Settings.

Theme selection must persist across application restarts.

Support a system-based theme preference when practical.

---

# 5. System Architecture

Use a lightweight client-agent architecture.

```text
                    PRIVATE TAILSCALE NETWORK

    ┌──────────────────────────────────────────┐
    │                                          │
    │         Android Device                  │
    │                                          │
    │          ANSHIN APP                     │
    │        Kotlin + Compose                 │
    │                                          │
    └───────────────────┬──────────────────────┘
                        │
                Encrypted Tailnet
                        │
            ┌───────────┴───────────┐
            │                       │
            ▼                       ▼
    ┌────────────────┐     ┌────────────────┐
    │ Linux Machine 1│     │ Linux Machine 2│
    │                │     │                │
    │  Anshin Agent  │     │  Anshin Agent  │
    │       Go       │     │       Go       │
    │                │     │                │
    └────────────────┘     └────────────────┘
```

There must be no central monitoring backend.

The Android application connects directly to each Linux agent using its Tailscale IP address or MagicDNS hostname.

Each Linux agent independently collects system metrics and exposes a minimal read-only API.

## Android stack

Use:

- Kotlin
- Jetpack Compose
- Material 3 foundations
- MVVM or another simple, maintainable presentation architecture
- Kotlin Coroutines
- StateFlow
- Retrofit or Ktor Client
- OkHttp where appropriate
- DataStore for non-sensitive preferences
- Android Keystore-backed storage for authentication credentials
- Dependency injection only if it materially improves maintainability

Prefer lightweight dependencies.

Avoid unnecessary abstraction layers and complex frameworks.

## Linux agent stack

Use:

- Go
- Standard library HTTP server
- gopsutil for system metrics where appropriate
- Minimal external dependencies
- systemd for service management

Build standalone binaries for:

- Linux AMD64
- Linux ARM64

The agent must run as an unprivileged user.

It must not require root privileges for normal monitoring.

---

# 6. Networking and Security

Security is a fundamental product requirement.

All monitoring communication must remain within the user's private Tailscale network.

## Network exposure

- Never expose agent APIs to the public Internet.
- Never listen on `0.0.0.0`.
- Bind exclusively to the machine's assigned Tailscale IPv4 address.
- Fail closed if the Tailscale interface or IP address is unavailable.
- Do not silently fall back to another interface.
- Do not use Tailscale Funnel.
- Do not configure public reverse proxies.
- Do not require public DNS.
- Do not require port forwarding.

Support MagicDNS hostnames for connecting to agents.

Handle Tailscale address changes and temporary disconnections gracefully.

Document how to restrict access using Tailscale grants or ACLs.

Provide a least-privilege policy example restricting the Android device to the agent port on the two Linux machines.

Explain that Tailscale policy enforcement depends on the user's actual tailnet configuration and device identities.

## Authentication

Implement additional per-agent authentication.

Requirements:

- Securely generated random bearer tokens.
- Separate token for each Linux machine.
- Constant-time token verification.
- Secure token storage on Android.
- Restricted permissions for agent configuration files.
- Never log tokens.
- Never include credentials in URLs.
- Do not expose authentication tokens in error messages.

For the MVP, HTTP over Tailscale's encrypted WireGuard transport is acceptable within the explicitly documented trust model.

Explain that Tailscale transport encryption does not replace application-layer TLS in every threat model.

Support adding HTTPS later without redesigning the networking layer.

## API restrictions

The API must be read-only.

Do not implement:

- Remote shell access.
- Command execution.
- Process termination.
- System configuration changes.
- File browsing.
- File uploads.
- Privileged administrative operations.

The monitoring agent should have the minimum permissions required to collect metrics.

## Android network configuration

- Explicitly allow cleartext HTTP only for the intended private-agent connections where Android platform configuration requires it.
- Do not broadly permit arbitrary cleartext traffic without justification.
- Validate configured endpoints.
- Do not follow redirects to unexpected destinations.
- Never silently fall back to public addresses.
- Handle IPv4 and MagicDNS resolution safely.

Document the security limitations of application-layer endpoint validation and use Tailscale policy enforcement as a primary network access control.

---

# 7. Linux Metrics Collection

Collect the following metrics efficiently.

## CPU

- Overall CPU usage percentage
- Per-core CPU usage
- Logical CPU count
- Load averages
- CPU temperature when available

## Memory

- Total RAM
- Used RAM
- Available RAM
- Memory usage percentage
- Total swap
- Used swap

## Storage

- Mounted filesystems
- Mount point
- Filesystem type
- Total capacity
- Used capacity
- Available capacity
- Usage percentage
- Disk read throughput when available
- Disk write throughput when available

Exclude irrelevant pseudo-filesystems from the default dashboard.

Allow advanced users to inspect additional mounts.

## Network

- Interface name
- Upload rate
- Download rate
- Total bytes transmitted
- Total bytes received
- Interface status

Handle counter resets and network interface changes correctly.

## System

- Hostname
- Linux distribution
- Kernel version
- Architecture
- System uptime
- Agent version
- Timestamp of last metric sample

## Metric sampling

Use a lightweight internal sampler.

Recommended defaults:

- Sample metrics every 5 seconds.
- Cache the latest snapshot.
- Serve API requests from the cached snapshot.
- Avoid expensive collection on every HTTP request.
- Use bounded memory.
- Avoid unnecessary disk writes.
- Do not persist monitoring history on the agent in the MVP.

Handle unavailable sensors and unsupported metrics gracefully.

Do not represent missing measurements as zero.

---

# 8. REST API

Implement a versioned API.

Suggested endpoints:

```http
GET /api/v1/health
GET /api/v1/system
GET /api/v1/metrics
```

All endpoints must require authentication, including health checks, unless a specific unauthenticated endpoint is explicitly justified.

Use JSON responses with stable field names.

Example:

```json
{
  "timestamp": "2026-11-10T12:00:00Z",
  "hostname": "arch-workstation",
  "cpu": {
    "usage_percent": 23.5,
    "cores": [18.2, 29.1, 21.4, 25.3]
  },
  "memory": {
    "total_bytes": 17179869184,
    "used_bytes": 8589934592,
    "usage_percent": 50.0
  },
  "uptime_seconds": 86400
}
```

Extend the schema to include all required metrics.

Use:

- Explicit units
- UTC timestamps
- Nullable fields for unavailable measurements
- Consistent error responses
- API versioning
- Reasonable request timeouts

Provide an OpenAPI specification.

---

# 9. Android Application UX

The Android experience is a major product differentiator.

Build an interface that feels as thoughtfully designed as a premium developer tool.

The application should be fast, calm, and extremely easy to understand.

## Main dashboard

Display all configured Linux machines.

Optimize the initial layout for two machines.

Each machine should appear in a compact, beautifully designed panel.

Show:

- Friendly hostname
- Online/offline status
- Tailscale IP or MagicDNS name
- CPU usage
- Memory usage
- Primary disk usage
- Last successful update

Use compact progress bars and clear numerical values.

The user should be able to understand the state of both machines immediately.

## Example visual structure

```text
┌──────────────────────────────────┐
│ anshin.                   ◉ TAIL │
│ SYSTEM OVERVIEW                  │
├──────────────────────────────────┤
│ arch-workstation        ● ONLINE │
│ 100.x.x.12                       │
│                                  │
│ CPU       MEM        DISK        │
│ 23%       58%        42%         │
│ ▰▰▱▱▱    ▰▰▰▱▱     ▰▰▱▱▱       │
├──────────────────────────────────┤
│ homelab                 ● ONLINE │
│ 100.x.x.18                       │
│                                  │
│ CPU       MEM        DISK        │
│  7%       34%        71%         │
│ ▰▱▱▱▱    ▰▰▱▱▱     ▰▰▰▰▱       │
├──────────────────────────────────┤
│ ALL SYSTEMS OPERATIONAL          │
│ LAST UPDATE: 2S AGO              │
└──────────────────────────────────┘
```

This is only a conceptual structure.

Create a polished native Compose implementation rather than reproducing ASCII layouts literally.

## Server details

Tapping a machine opens a detailed monitoring screen.

Include:

### Overview
- System information
- Uptime
- Connectivity
- Agent version

### CPU
- Current usage
- Per-core usage
- Short-term history chart
- Load averages
- Temperature

### Memory
- RAM usage
- Available memory
- Swap usage
- Short-term history

### Storage
- Filesystem list
- Usage bars
- Capacity values
- Disk I/O rates

### Network
- Upload/download throughput
- Traffic charts
- Interface information

Charts should be compact, smooth, readable, and efficient.

Use lightweight drawing primitives or Compose Canvas when appropriate.

Avoid adding a large charting dependency unless justified.

## Server management

Allow users to:

- Add a Linux machine manually.
- Enter a Tailscale IP or MagicDNS hostname.
- Configure the agent port.
- Enter the authentication token.
- Assign a friendly display name.
- Test connectivity.
- Edit a machine.
- Remove a machine.

Manual setup is sufficient for the MVP.

Automatic discovery is optional and should not delay implementation.

## Settings

Provide:

- Theme selection
- Refresh interval
- Server management
- About Anshin
- App version
- Open-source licenses

Keep settings simple and well organized.

---

# 10. Refresh, State, and History

## Polling

Default refresh interval: 5 seconds.

Allow:

- 2 seconds
- 5 seconds
- 10 seconds
- 30 seconds
- 60 seconds

Poll each machine independently.

Use structured concurrency and cancellation-aware requests.

## Background behavior

- Poll only while the app is actively visible.
- Stop polling when the app enters the background.
- Resume gracefully when it returns.
- Do not run persistent background services.
- Do not use wake locks for continuous monitoring.

Battery efficiency is essential.

## Historical data

For the MVP:

- Keep bounded in-memory samples.
- Show short-term charts.
- Use a configurable or fixed rolling window, such as 5–15 minutes.
- Do not require a database.
- Do not persist historical metrics across app restarts.

Design the data model so persistent history could be introduced later.

## Offline handling

Distinguish clearly between:

- Online
- Connecting
- Offline
- Authentication failed
- Stale metrics
- Agent unavailable
- Tailscale unreachable

Do not display old metrics as if they were current.

Show the last successful update timestamp.

Use sensible connection timeouts and exponential backoff.

---

# 11. Performance Requirements

Anshin must be lightweight.

## Linux agent

Target:

- Minimal idle CPU usage.
- Small memory footprint.
- No unnecessary allocations.
- No continuous disk writes.
- No heavy monitoring framework.
- No external database.

Prefer simple sampling loops and cached metric snapshots.

## Android

Target:

- Fast startup.
- Smooth scrolling.
- Efficient recomposition.
- Low battery consumption.
- Minimal background activity.
- Small APK size.
- No unnecessary SDKs.

Avoid introducing dependencies without a clear benefit.

Measure and document actual resource usage instead of claiming unverified performance numbers.

---

# 12. Accessibility and Responsiveness

Support:

- Small Android phones.
- Large phones.
- Tablets where practical.
- Portrait orientation.
- Landscape orientation where useful.
- Dynamic font scaling.
- Accessible contrast.
- Screen readers.
- Reduced-motion preferences.
- Minimum practical touch target sizes.

The visual style may be compact, but accessibility must not be sacrificed.

Use color and text together for system states.

Never rely on color alone to indicate failures.

---

# 13. Repository Structure

Use a clean monorepo.

Suggested structure:

```text
anshin/
├── android/
│   ├── app/
│   ├── build.gradle.kts
│   └── settings.gradle.kts
│
├── agent/
│   ├── cmd/
│   │   └── anshin-agent/
│   ├── internal/
│   │   ├── api/
│   │   ├── auth/
│   │   ├── metrics/
│   │   ├── sampler/
│   │   └── config/
│   ├── go.mod
│   └── go.sum
│
├── deploy/
│   ├── systemd/
│   ├── scripts/
│   └── tailscale/
│
├── docs/
│   ├── architecture.md
│   ├── security.md
│   ├── installation.md
│   └── api.yaml
│
├── branding/
│   ├── icons/
│   └── themes/
│
├── README.md
└── LICENSE
```

Adjust the structure when justified, but keep it straightforward.

---

# 14. Linux Agent Installation

Provide a simple installation process.

Requirements:

- Install the standalone Go binary.
- Generate a secure authentication token.
- Create a restricted configuration file.
- Detect the machine's Tailscale IPv4 address.
- Configure the agent to bind exclusively to that address.
- Install and enable a systemd service.
- Restart safely after failures.
- Log useful diagnostic information without exposing secrets.

Provide:

- Installation script
- Uninstallation script
- systemd service template
- Example configuration
- Troubleshooting guide

Do not silently change firewall rules or Tailscale policies.

Provide clear instructions for required manual security configuration.

Avoid piping unverified remote scripts directly into a privileged shell.

---

# 15. Testing

Implement meaningful automated tests.

## Go agent

Test:

- Metric collection
- Unavailable sensors
- API authentication
- Invalid tokens
- Token comparison
- API responses
- Sampling behavior
- Network binding behavior
- Failure when Tailscale is unavailable
- Graceful shutdown

## Android

Test:

- Server configuration
- Authentication handling
- JSON parsing
- Connection failures
- Polling lifecycle
- Offline states
- Theme switching
- ViewModel state management

Provide Compose previews for:

- Online server
- Offline server
- Loading state
- Authentication error
- Two-server dashboard
- Each supported theme

Do not rely only on screenshots or manually verified behavior.

---

# 16. Documentation

Create a clear README explaining:

1. What Anshin is.
2. Its privacy-first architecture.
3. System requirements.
4. How to install the Linux agent.
5. How to configure Tailscale access.
6. How to build and install the Android app.
7. How to connect the first Linux machine.
8. How to add a second machine.
9. How to troubleshoot connectivity.
10. How to update or uninstall the agent.

Include a security section explaining the trust model.

Document that binding to a Tailscale address and configuring tailnet access policies are separate security controls.

Include example configurations without real credentials.

---

# 17. Deliverables

Deliver:

1. A complete Android Studio project.
2. A working Go monitoring agent.
3. Linux AMD64 and ARM64 build support.
4. A functional, polished Compose dashboard.
5. Detailed server monitoring screens.
6. Four selectable visual themes.
7. Original Anshin branding assets.
8. Secure server configuration and token storage.
9. systemd service configuration.
10. Installation and uninstallation scripts.
11. Example Tailscale grants/ACL policy.
12. OpenAPI specification.
13. Automated tests.
14. Complete setup documentation.
15. Reproducible build instructions.

Include a suitable open-source license, with MIT as the default unless dependencies or assets require a different approach.

---

# 18. Implementation Strategy

Work incrementally, but implement the actual product.

**Phase 1 — Foundation**

- Inspect the repository and existing environment.
- Propose the architecture.
- Define the security model.
- Define the API contract.
- Establish the repository structure.

**Phase 2 — Linux Agent**

- Implement system metrics collection.
- Implement the sampling loop.
- Implement authentication.
- Implement the HTTP API.
- Implement safe Tailscale binding.
- Write automated tests.

**Phase 3 — Android Foundation**

- Create the Android project.
- Implement the theme engine.
- Implement navigation.
- Implement server configuration.
- Implement secure credential storage.
- Implement API communication.

**Phase 4 — Dashboard**

- Build the main monitoring screen.
- Implement live polling.
- Implement offline and stale states.
- Build compact resource indicators.
- Implement detailed server views.
- Implement lightweight charts.

**Phase 5 — Branding and Polish**

- Finalize the Anshin logo.
- Implement all four themes.
- Refine typography and spacing.
- Improve transitions and accessibility.
- Optimize performance.

**Phase 6 — Validation and Delivery**

- Run automated tests.
- Verify the Android build.
- Verify Go cross-compilation.
- Validate security assumptions.
- Prepare installation scripts.
- Complete documentation.

---

# 19. Engineering Guidelines

Follow these rules throughout development:

- Prefer simple, readable code.
- Use idiomatic Kotlin and Go.
- Avoid unnecessary abstractions.
- Avoid premature optimization.
- Keep dependencies minimal.
- Use secure defaults.
- Handle errors explicitly.
- Never hide failed operations.
- Do not fabricate test results.
- Do not claim features are complete unless implemented.
- Keep implementation decisions documented.
- Maintain consistent naming and formatting.
- Prioritize reliability over feature quantity.

Do not introduce:

- Cloud infrastructure.
- User accounts.
- Subscription systems.
- Analytics.
- Advertisements.
- Remote shell functionality.
- Docker orchestration.
- Process management.
- Push notification services.
- Persistent monitoring databases.

These are outside the initial MVP scope.

---

# 20. Definition of Done

The MVP is complete when:

- Anshin can be installed on an Android device.
- The Linux agent can be installed on two separate Linux computers.
- Both agents listen exclusively on their assigned Tailscale addresses.
- The Android app connects securely to both agents through the tailnet.
- Both machines appear on the main dashboard.
- CPU, RAM, storage, network, uptime, and available temperature metrics are displayed correctly.
- The user can open detailed monitoring screens.
- Metrics refresh automatically while the app is visible.
- Offline and authentication errors are handled correctly.
- The user can change themes.
- The application works without Internet access when the tailnet connectivity is available.
- No public monitoring endpoint is exposed.
- The agent runs without root privileges.
- Automated tests pass.
- Installation instructions are complete and reproducible.

---

# 21. Final Instructions

(Original agent instructions omitted: this brief is now an input to design,
not a build order.)

**Anshin — Your systems. At peace.**
