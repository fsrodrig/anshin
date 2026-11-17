# Anshin's security model

This page is for someone who doesn't take "only your own devices get in" on faith. It says what Anshin trusts, how the Agent decides who gets a Pulse, what the phone refuses to do, and what is left over. Every claim links to the code or test that enforces it; where nothing enforces something, the page says so.

The model itself is defined in [design § Security model](design.md#security-model), and the reasons for it are in [ADR 0002](adr/0002-tailnet-identity-no-tokens.md). Where this page and the design disagree, the design wins. Words such as **Machine**, **Agent**, **Owner** and **Pulse** mean what [the glossary](../GLOSSARY.md) says.

## Not yet verified on real hardware

The implementation controls below are checked by tests that run against a fake tailscaled, loopback sockets and stand-in servers. Trust assumptions are not enforced or testable by Anshin. Nobody has yet recorded the bind on a real Machine (for example with `ss -ltnp`) or an end-to-end run from a real phone; that validation is still open. The repository also doesn't yet ship a systemd unit or a package, so nothing here depends on one.

## What is trusted

- **tailscaled on each Machine.** The Agent believes what the local tailscaled tells it: this Machine's own addresses, user and tags ([`LocalAPI.Self`](../agent/internal/tailnet/localapi.go)), and who owns the address a connection comes from ([`LocalAPI.WhoIs`](../agent/internal/tailnet/localapi.go)). The client for those two read-only calls is tested against a stub daemon shaped like tailscaled's answers ([`TestSelf`, `TestWhoIs`](../agent/internal/tailnet/localapi_test.go)).
- **Whoever stands behind tailscaled's answers.** tailscaled's idea of which user owns which device and which devices carry tags comes from the tailnet's coordination server, and from whoever administers the tailnet, since they add users and assign tags. Anshin can't check those answers; if the coordination server or a tailnet admin lies about who a device belongs to, the Agent believes the lie. Anshin adds no second identity layer of its own ([ADR 0002](adr/0002-tailnet-identity-no-tokens.md)).
- **The Owner's untagged devices.** Any untagged tailnet node created by the same user as the Machine gets a Pulse ([`TestTruthTable`](../agent/internal/server/server_test.go), row "same user, caller untagged, Machine untagged: allow"). That includes the Owner's laptop, other Machines, and every app on the Owner's phone; see [residual risks](#residual-risks). The [tailnet policy](#three-separate-controls) can narrow it further.
- **Root on each Machine.** Trusting root is an assumption that nothing in Anshin enforces or can test. Root can change what the Agent reads, replace the Agent, or answer in its place. The Agent reads the Machine's filesystem through [`liveHost`](../agent/cmd/anshin-agent/host.go), which builds `probe.Host{FS: os.DirFS("/"), …}`, and the [`probe` package](../agent/internal/recorder/probe/).

## How the Agent decides who gets a Pulse

### It listens only on its tailnet addresses

At start the Agent asks tailscaled for this Machine's tailnet addresses and binds each one, and nothing else ([`Run` and `Listen`](../agent/internal/server/server.go); [`TestListenBindsOnlyTheGivenAddresses`](../agent/internal/server/server_test.go)). It sets `IP_FREEBIND` so the bind works before tailscaled has put the addresses on its interface ([`freebind`](../agent/internal/server/freebind_linux.go); [`TestListenBindsAddressesNotYetOnAnInterface`](../agent/internal/server/server_test.go)). On a kernel with IPv6 disabled it serves on the IPv4 address alone, and any other bind failure stops it ([`TestListenSkipsIPv6OnlyWhenTheKernelHasNone`](../agent/internal/server/server_test.go)).

Binding chooses local destination addresses, not ingress interfaces ([`Listen`](../agent/internal/server/server.go)). A host on the same network that routes packets to the Machine's tailnet address can still reach the socket. If its source address is not a tailnet peer, the truth table refuses it with 403 ([`Server.authorize`](../agent/internal/server/server.go); [`TestTruthTable`](../agent/internal/server/server_test.go), "not a tailnet peer"). Dropping such packets before they reach the Agent is a host firewall's job. Anshin configures no host firewall ([`run`](../agent/cmd/anshin-agent/main.go)).

The listen address can't be configured: `ANSHIN_PORT` sets the port, and there is no setting for the address ([`run` in `main.go`](../agent/cmd/anshin-agent/main.go)).

If tailscaled can't say who this Machine is at start, the Agent exits with an error ([`TestRunFailsWhenSelfFails`](../agent/internal/server/server_test.go)). It also exits when tailscaled reports no user for this Machine ([`LocalAPI.Self`](../agent/internal/tailnet/localapi.go); [`TestSelfFailsWithoutAUser`](../agent/internal/tailnet/localapi_test.go)). It asks again every 30 s, and exits non-zero when the addresses, the MagicDNS name, the user or the tags have changed, so that whatever supervises it starts it again with the new identity ([`watchSelf`](../agent/internal/server/server.go); [`TestRunExitsWhenIdentityChanges`, `TestRunExitsWhenThisMachineIsTaggedRenamedOrChangesUser`](../agent/internal/server/server_test.go)). Between a change and that recheck, up to about 30 s, the Agent still answers with its old identity.

### Every request is authorized before anything else

For every request, before it looks at the path or the method, the Agent identifies the connection's source address through WhoIs or a fresh cached answer and applies a truth table ([`Server.authorize` and `Server.whoIs`](../agent/internal/server/server.go)). This is the only place authorization is decided.

The identity comes from the connection's source address as a `netip.Addr`, never from the port or anything the caller sends. Headers that proxies use to name a client (`X-Forwarded-For`, `X-Real-IP`, `Forwarded` and similar) change nothing ([`TestHeadersCannotChangeWhoTheCallerIs`](../agent/internal/server/server_test.go)).

This table mirrors the one in [design § Agent side](design.md#agent-side). Each row names the [`TestTruthTable`](../agent/internal/server/server_test.go) cases that check it:

| Caller same user as this Machine's Owner | Caller tagged | This Machine tagged | Result | `TestTruthTable` cases |
|---|---|---|---|---|
| yes | no | no | **allow** | "same user, caller untagged, Machine untagged: allow" |
| yes | yes | any | 403 | "same user, caller tagged, Machine untagged"; "same user, caller tagged, Machine tagged" |
| yes | any | yes | 403 | "same user, caller untagged, Machine tagged"; "same user, caller tagged, Machine tagged" |
| no | any | any | 403 | the four "other user, …" cases |
| WhoIs says "not a tailnet peer" | | | 403 | "not a tailnet peer" |
| LocalAPI unreachable or erroring | | | 503 | "LocalAPI unreachable or erroring" |
| no user (ID 0) | | | 403 | "no user on either side" |

Tags deny on either side because tailscaled reports, for a tagged tailnet node, the user who created it, which says nothing about who owns it now ([`Node`](../agent/internal/tailnet/tailnet.go)). No tailnet user has ID 0, so if tailscaled's answers ever stopped decoding, a 0 on both sides can't make every caller the Owner ([`Allows`](../agent/internal/server/server.go)). The phone shows a 403 as **Forbidden** ([`AgentLinkTest.http403IsForbidden`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)).

When a WhoIs lookup has to reach LocalAPI and fails, the Agent answers 503 and does not cache the error, so the next request asks again ([`Server.whoIs`](../agent/internal/server/server.go); [`TestNotPeerIsCachedOnlyBrieflyAndLocalAPIErrorsAreNot`](../agent/internal/server/server_test.go)). Cached answers stand for their remaining time even during a LocalAPI outage: an allowed identity keeps getting Pulses until its roughly 30 s entry expires, and a cached "not a tailnet peer" keeps getting 403 for up to roughly 5 s ([`Server.whoIs`](../agent/internal/server/server.go); [`TestWhoIsIsCachedByAddressForAboutThirtySeconds`, `TestNotPeerIsCachedOnlyBrieflyAndLocalAPIErrorsAreNot`](../agent/internal/server/server_test.go)). A LocalAPI error is never mistaken for "not a tailnet peer" ([`TestWhoIsDaemonErrorIsNotNotPeer`, `TestFailsWithoutDaemon`](../agent/internal/tailnet/localapi_test.go)).

WhoIs answers are cached by address ([`Server.whoIs`](../agent/internal/server/server.go)):

- An identity is kept for about 30 s ([`TestWhoIsIsCachedByAddressForAboutThirtySeconds`](../agent/internal/server/server_test.go)). So a caller that is tagged or removed keeps getting Pulses for up to about 30 s; that test shows a retagged caller still allowed at 29 s and refused at 31 s.
- A "not a tailnet peer" answer is kept for about 5 s, because a restarting tailscaled says that of every address until it has loaded the tailnet ([`TestNotPeerIsCachedOnlyBrieflyAndLocalAPIErrorsAreNot`](../agent/internal/server/server_test.go)).
- A LocalAPI error isn't cached at all (same test).

A refused caller learns nothing about the API: whatever path or method it tries, it gets 403 ([`TestRefusedCallersLearnNothingAboutRoutes`](../agent/internal/server/server_test.go)), and whatever `Host` it sends ([`TestAStrangerIsRefusedWhateverTheHost`](../agent/internal/server/server_test.go)). An allowed caller can only `GET /v1/pulse`; other methods get 405 and other paths 404 ([`TestOnlyGET`](../agent/internal/server/server_test.go)).

Headers arrive before WhoIs can refuse anyone, so the Agent reads at most 8 KiB of them plus Go's own 4 KiB allowance; more gets 431 without a WhoIs ([`maxHeaderBytes`](../agent/internal/server/server.go); [`TestOversizedHeadersAreRefusedBeforeWhoIs`](../agent/internal/server/server_test.go)).

### It answers only requests that name this Machine

WhoIs identifies a device, not a program on it. A web page open in a browser on the Owner's laptop could have its domain rebound to this Machine's tailnet address: the browser then connects from the laptop, WhoIs allows it, and the page could read the Pulse as same-origin. That browser sends the page's domain as `Host`, so after authorizing a caller the Agent answers 421 unless `Host` is one of this Machine's tailnet addresses, its MagicDNS name or that name's first label, ignoring case, a port and a name's trailing dot ([`Server.isSelf`](../agent/internal/server/server.go); [`TestHostMustNameThisMachine`, `TestAnHTTP10RequestWithoutHostIsMisdirected`, `TestOnlyAddressesNameAMachineWithoutMagicDNS`](../agent/internal/server/server_test.go)). The phone sends the address it was given, so a Machine has to be added by one of those; one reached by a custom DNS name gets 421, and the phone says to use the tailnet address or MagicDNS name.

## Why there are no tokens

There are no bearer tokens, passwords or client certificates, and the phone stores none. The argument, from [ADR 0002](adr/0002-tailnet-identity-no-tokens.md):

- WireGuard already authenticates every tailnet node, and tailscaled already knows who owns each address. A token would be a second secret to generate, copy, store and rotate that protects against almost nothing on top of that.
- If the bind were ever wrong, a token check would fail open to the LAN, where WhoIs fails closed: an address that isn't a tailnet node gets 403 ([`TestTruthTable`](../agent/internal/server/server_test.go), "not a tailnet peer").
- Because the phone sends no secret, reaching the wrong server at a tailnet-range address leaks nothing (see [residual risks](#residual-risks)).

## What the phone enforces

The phone talks to Agents through one OkHttp client, [`TailnetHttp.client`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/TailnetHttp.kt), which the app hands to [`AgentLink`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/AgentLink.kt) in [`MainActivity`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/MainActivity.kt). No test proves the app never grows a second client; code review holds that line. The client is configured like this:

- **A destination check in the socket.** Every socket the client opens refuses to connect unless the target is a resolved address in `100.64.0.0/10` or `fd7a:115c:a1e0::/48` ([`requireTailnet` and `TailnetSocket`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/TailnetHttp.kt)). The check runs on the address being connected to, after any name lookup, so it covers MagicDNS names and IP literals alike. Tests in [`DestinationCheckTest`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/DestinationCheckTest.kt) cover both edges of each range (`ipv4InsideTheRangeIsAllowedUpToBothEdges`, `ipv4JustOutsideTheRangeIsRefused`, `ipv6InsideTheRangeIsAllowedUpToBothEdges`, `ipv6OutsideTheRangeIsRefused`), an unresolved address (`anUnresolvedAddressIsRefusedEvenWhenItsNameLooksLikeATailnetAddress`), and every way to connect a socket (`theOneArgumentConnectIsChecked`, `theTwoArgumentConnectIsChecked`, `theFactoryChecksSocketsItConnectsItself`).
- **No proxy.** `Proxy.NO_PROXY` keeps a configured system proxy from carrying a request somewhere the check never sees. [`ProxyBypassTest`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/ProxyBypassTest.kt) sets up a live system proxy and shows an ordinary client uses it (`theProxyIsLiveForAnOrdinaryClient`) while Anshin's doesn't, for an outside address (`anOutsideAddressIsRefusedAndNeverReachesTheProxy`, `anOutsideAddressIsUnreachableThroughTheAgentLink`) or a tailnet one (`aTailnetAddressGoesDirectlyNotThroughTheProxy`).
- **A capped answer.** A Pulse body over 1 MiB, counted after OkHttp inflates gzip, is Incompatible. Decoding the most expensive hostile 1 MiB body measured needed 53 MiB of heap at its peak, on the JVM rather than ART ([measurements](measurements.md#phone)), so the worst answer tested from a server at a tailnet-range address needed about that on the JVM, however small it was compressed. Each watched Machine's answer is decoded on its own, so several hostile answers at once each cost as much; see [residual risks](#residual-risks) ([`MAX_PULSE_BYTES`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/AgentLink.kt); [`AgentLinkTest.aPulseAsLargeAsTheCapIsOk`, `aBodyOverTheCapIsIncompatible`, `aGzipBodyThatInflatesOverTheCapIsIncompatible`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)).
- **No redirects.** A redirect is reported, never followed, so an answer can't send the phone somewhere else ([`AgentLinkTest.aRedirectIsNotFollowed`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)).
- **No credential, nothing about the phone.** A request carries only `Host`, `Connection`, `Accept-Encoding` and OkHttp's own `User-Agent`, which names the library and its version: no `Authorization`, no cookie, nothing about the phone ([`AgentLinkTest.aRequestCarriesNoCredentialAndNothingAboutThePhone`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)).
- **Cleartext, only because the platform forces the choice.** Agents speak plain HTTP; on the tailnet it travels inside WireGuard. The app allows cleartext app-wide ([`network_security_config.xml`](../android/app/src/main/res/xml/network_security_config.xml)) because Android's network security config can't match IP literals or address ranges, so it can't allow cleartext only to the tailnet. The destination check is what keeps those connections on the tailnet ranges.

## Three separate controls

Anshin's bind, your tailnet's access policy and the Agent's truth table each answer a different question. None replaces another.

| Control | Decides | Enforced by |
|---|---|---|
| The Agent's bind | Which local addresses the Agent accepts connections on: only its tailnet addresses, never a wildcard or LAN address | the Agent, [`Listen`](../agent/internal/server/server.go); [`TestListenBindsOnlyTheGivenAddresses`](../agent/internal/server/server_test.go) |
| The tailnet access policy | Which tailnet nodes' packets reach the Agent's port | tailscaled on the Machine, from the policy file the tailnet admin writes |
| The truth table | Which of the callers that got through receive a Pulse | the Agent, [`Server.authorize`](../agent/internal/server/server.go) |

The Agent can only tell "the Owner's untagged devices" from everyone else. The tailnet policy can say more, such as "only the Owner's phone", which the Agent alone cannot. Tailscale enforces the policy on the receiving device, so the Machine's tailscaled drops packets the policy doesn't grant before they reach the Agent ([Tailscale: Manage permissions using ACLs](https://tailscale.com/kb/1018/acls)). The policy governs tailnet traffic; it does not replace a host firewall for packets arriving outside the tailnet ([see the bind limits above](#it-listens-only-on-its-tailnet-addresses)). It also doesn't replace the truth table: a grant to a tagged device or to another user's device still gets 403 from the Agent ([`TestTruthTable`](../agent/internal/server/server_test.go)).

### An example policy

[`deploy/tailscale/policy.example.hujson`](../deploy/tailscale/policy.example.hujson) is a least-privilege grant that lets only the Owner's phone reach TCP 7247 on two Machines. It depends on your own tailnet: every address in it is a placeholder.

- Machines and the phone are untagged, because the Agent refuses tagged ones, so the example can't select them by tag. A user selector such as `owner@example.com` would select all of the Owner's devices, which is what the Agent already allows. So the example names the phone and the Machines by their tailnet addresses, gathered into IP sets. Tailscale turns an IP set into a list of address ranges, so both the IPv4 and the IPv6 address of each device are listed.
- Its `tests` section asks Tailscale to reject a policy change under which the phone can't reach the Machines, or another of your devices can.
- **Merge it into your tailnet policy file; don't paste it over the file.** A policy denies whatever it doesn't grant, so this file alone would cut off every other connection on your tailnet. And merging it narrows nothing while another rule still opens the port: the policy Tailscale creates for a new tailnet lets every device reach every other. The example's deny test fails until that rule is gone or narrowed.

`make check-repo` parses the file with Tailscale's own HuJSON parser and checks that it is in that parser's canonical format ([`scripts/hujson-check.sh`](../scripts/hujson-check.sh)), and the identifier scan keeps real addresses out of it ([`scripts/identifier-scan.sh`](../scripts/identifier-scan.sh)). Neither can check it against Tailscale's policy rules; only Tailscale can, when you save the merged policy.

The example follows Tailscale's [grants syntax](https://tailscale.com/kb/1538/grants-syntax), [policy file syntax](https://tailscale.com/kb/1337/policy-syntax) (hosts and tests) and [IP sets](https://tailscale.com/docs/features/tailnet-policy-file/ip-sets).

## Residual risks

These are accepted for the MVP, and listed in [design § Residual risks](design.md#residual-risks) too.

- **Identity is per device, not per app.** The Agent sees a tailnet node, not an app, so any other app on the Owner's phone can read the same Pulses. The data is read-only and the API has no way to change anything on the Machine ([`TestOnlyGET`](../agent/internal/server/server_test.go)). The later fix is device-key pairing, which isn't in the MVP ([design § Not in the MVP](design.md#not-in-the-mvp)).
- **Whoever controls a browser's DNS can still name the Machine.** The `Host` check stops a web page from reaching a Machine under the page's own domain ([see above](#it-answers-only-requests-that-name-this-machine)). It can't stop one served under the Machine's own short or MagicDNS name, which takes control of the DNS resolver or search domain of a browser on the Owner's device, for instance on a hostile network with Tailscale DNS turned off on that device.
- **A hostile answer can still crash a phone with little heap.** The worst answer tested under the response cap needed about 53 MiB of heap at its peak on the JVM ([see above](#what-the-phone-enforces)). Most phones give an app far more, but Android's compatibility definition only guarantees each app 48 MB on a small or normal screen at 180–280 dpi, 80 MB at 320 dpi and 128 MB at 480 dpi ([Android 11 CDD § 3.7](https://source.android.com/docs/compatibility/11/android-11-cdd#3_7_runtime_compatibility)), so on a device near that minimum one hostile answer, or several at once, can still run the app out of memory. It can't make the phone send anything.
- **On a carrier that uses CGNAT, a `100.64.0.0/10` address doesn't prove the tailnet.** The destination check only proves the address is in the tailnet's range ([`TailnetAddresses`](../android/app/src/main/kotlin/io/github/fsrodrig/anshin/TailnetHttp.kt)). Some mobile carriers use the same range on their own networks, so with the tailnet down the phone could reach a stranger at that address. That is acceptable only because the phone sends no secret and nothing about itself ([`aRequestCarriesNoCredentialAndNothingAboutThePhone`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)) and follows no redirect ([`aRedirectIsNotFollowed`](../android/app/src/test/kotlin/io/github/fsrodrig/anshin/AgentLinkTest.kt)). The worst case is fake numbers, including a reassuring Verdict that isn't true.
- **Root on a Machine can falsify anything that Machine reports.** The Agent reads `/proc` and `/sys` through [`liveHost`](../agent/cmd/anshin-agent/host.go) and the [`probe` package](../agent/internal/recorder/probe/) and reports what it finds. Trusting root not to falsify those readings is an assumption that nothing in Anshin enforces or can test.
- **Revocation takes up to about 30 s.** A caller that has just been tagged or removed from the tailnet keeps getting Pulses until its cached WhoIs answer expires, and a Machine that has just been tagged keeps answering until its next identity recheck ([see above](#how-the-agent-decides-who-gets-a-pulse)).
- **The trust in tailscaled is total.** Anything that can make tailscaled misreport a caller's user or tags, whether a compromised coordination server, a tailnet admin, or root on the Machine, gets a Pulse.

[SECURITY.md](../SECURITY.md) treats these accepted risks as out of scope for vulnerability reports.
