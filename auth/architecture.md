# Authentication Architecture

Technical reference for the authentication machinery in this library.

## System Boundaries

The Edeka app talks to three independent APIs. Each has its own auth scheme. Credentials cannot be swapped between them.

| System              | Host                        | Auth                              | In this library? |
|---------------------|-----------------------------|-----------------------------------|------------------|
| SOAP / ValuePhone   | `www.myvaluephone.com`      | HTTP Basic (Username/Password)    | Yes - receipts, markets |
| Offers REST         | `b2c-gw.api.edeka`          | `Authorization: Bearer` (b2b)     | Yes - offers    |
| User SSO REST       | `login.edeka/api/...`       | `X-Authorization: Bearer` (user)  | No              |

The third api (Customer Service API under `login.edeka/api/customer-service/...`) uses a custom header name `X-Authorization` and the user's OIDC access token. It is not implemented here and contains market favourites, consent prefs, or Payback linking, which I do not find interesting.

## SOAP Credential Acquisition

The SOAP API uses a Basic-auth username/password pair minted by the ValuePhone server. Acquiring it is a four-step dance:

| # | SOAP action                                    | Endpoint service                    | Purpose |
|---|------------------------------------------------|-------------------------------------|---------|
| 1 | `registerMobileApplicationForAnonymousUser`    | `OpenMobileDeviceRegistrationManager` | Mint temporary anon creds tied to DeviceConfig |
| 2 | `login`                                        | `MobileSSOServiceManager`           | Exchange OIDC bearer (from `login.edeka` user SSO) + anon creds → permanent creds |
| 3 | `checkMobileApplicationRegistrationLogin`      | `OpenMobileDeviceRegistrationManager` | Validate the permanent creds on the server |
| 4 | `notifyPostLogin`                              | `MobileUserManager`                 | Announce platform/version/country after login |

Shortcut paths skip steps depending on entry point:

- `RegisterAnonymously` → step 1 + 4 only. The resulting credentials are temporary and cannot be used for authenticated endpoints like receipts.
- `LoginWithCredentials` / `LoginFromAuthFile` → steps 3 + 4 only (skip the bearer exchange because creds are already in hand).
- `CredentialsFromBearer` / `RefreshCredentialsFromBearer` → full 1 → 2 → 3 → 4.

### Device binding

The server ties permanent credentials to the IMEI and serial number sent during step 1. Refreshing credentials with the same device identity reuses the existing registration. Using a different device identity each time creates a new device registration on the account.

The device identity is saved alongside the credentials in `edeka_auth.json`. This file contains plaintext Basic-auth credentials and should be treated as a secret.

## Offers Bearer (b2b client_credentials)

The offers REST API at `b2c-gw.api.edeka` requires a machine-to-machine token from a **different** Keycloak realm than the user login:

- **Token URL:** `https://b2b-login.api.edeka/auth/realms/b2b/protocol/openid-connect/token`
- **Grant:** `client_credentials`
- **Client auth:** HTTP Basic with a fixed client ID and secret extracted from the APK

```
POST /auth/realms/b2b/protocol/openid-connect/token HTTP/1.1
Authorization: Basic <base64(client_id:client_secret)>
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
```

The response contains an `access_token` and `expires_in` (observed TTL is short, on the order of a few minutes). The library caches the token and refreshes it automatically before expiry.

This bearer authenticates the **app**, not a user account. Fetching offers does not require SOAP credentials and works without logging in.

## User OIDC Token (PKCE)

The user login at `login.edeka` is a standard PKCE-protected OIDC flow against a third Keycloak realm (`b2c`):

| Field          | Value                                                          |
|----------------|----------------------------------------------------------------|
| Auth URL       | `https://login.edeka/auth/realms/b2c/protocol/openid-connect/auth`  |
| Token URL      | `https://login.edeka/auth/realms/b2c/protocol/openid-connect/token` |
| Logout URL     | `https://login.edeka/auth/realms/b2c/protocol/openid-connect/logout` |
| Client ID      | `edeka-app`                                                    |
| Redirect URI   | `edeka://main/auth/redirect` (custom scheme)                   |
| Scopes         | `profile email offline_access`                                 |
| Code challenge | S256                                                           |

The `edeka://` redirect URI is handled by the Edeka Android app via an intent filter - Keycloak redirects back to `edeka://main/auth/redirect?code=...` and the OS routes it to the app, which exchanges the code for tokens at the token endpoint.

This library does not run the PKCE flow. It consumes the resulting `access_token` once to mint SOAP credentials. The OIDC token is discarded after the exchange and not persisted. To obtain a bearer, use the [edekompile-auth-helper](https://github.com/ByteSizedMarius/edekompile-auth-helper) CLI which handles the full PKCE flow.

