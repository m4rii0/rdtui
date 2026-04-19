## MODIFIED Requirements

### Requirement: User can choose how the direct URL is handed off
After generating a direct download URL, the application SHALL allow the user to view or copy that URL without starting a managed local download.

#### Scenario: User chooses copy or show behavior
- **WHEN** the user requests a direct URL handoff for a ready torrent target
- **THEN** the application presents the direct URL to the user and, when supported, copies it to the clipboard

#### Scenario: Direct URL handoff stays separate from managed download
- **WHEN** the user chooses direct URL handoff instead of managed download
- **THEN** the application completes the URL display and copy flow without starting `aria2c`

## REMOVED Requirements

### Requirement: External downloader handoff is configurable and non-monitoring
**Reason**: Managed aria2 downloads replace the external-launch workflow, while direct URL copy and show remain available as the lightweight handoff path.

**Migration**: Existing `external_command` configuration is ignored by the new flow and may be removed from saved config the next time the application writes configuration.
