## ADDED Requirements

### Requirement: User can authenticate with Real-Debrid using device auth
The application SHALL allow a user to authenticate with Real-Debrid using the device authorization flow and SHALL persist the resulting user-bound credentials for reuse in later sessions.

#### Scenario: First-run device authentication succeeds
- **WHEN** the user chooses device authentication and completes the authorization flow successfully
- **THEN** the application stores the resulting credentials locally and enters the authenticated torrent UI without asking for a private API token

#### Scenario: Stored device credentials can be reused
- **WHEN** the user starts the application later and valid stored device-auth credentials exist
- **THEN** the application uses those credentials automatically and does not prompt for authentication again

### Requirement: User can authenticate with a private API token
The application SHALL allow a user to authenticate by providing a Real-Debrid private API token and SHALL validate that token before enabling torrent actions.

#### Scenario: Private token is valid
- **WHEN** the user provides a valid private API token
- **THEN** the application validates it against the Real-Debrid API and allows the user to continue into the torrent UI

#### Scenario: Private token is invalid
- **WHEN** the user provides an invalid or expired private API token
- **THEN** the application rejects the token, explains that authentication failed, and keeps the user in an authentication flow

### Requirement: Authentication sources have deterministic precedence
The application SHALL apply a deterministic credential precedence so that an explicitly configured private API token overrides stored device-auth credentials, and stored device-auth credentials override an unauthenticated first-run prompt.

#### Scenario: Configured token overrides stored device credentials
- **WHEN** a valid private API token is configured and valid stored device-auth credentials also exist
- **THEN** the application authenticates with the configured private API token instead of the stored device-auth credentials

#### Scenario: No valid credentials are available
- **WHEN** there is no valid configured token and no reusable device-auth credential set
- **THEN** the application shows an authentication prompt before any torrent-management action is available
