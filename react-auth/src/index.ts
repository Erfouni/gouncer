// SPDX-License-Identifier: Apache-2.0

export type { Account } from './account.js'
export {
	InvalidCredentialsError,
	InvalidTokenError,
	RateLimitedError,
	UnauthorizedError,
	ValidationError,
	acceptInvite,
	fetchSession,
	login,
	logout,
	requestPasswordReset,
	resetPassword,
} from './api.js'
export type { User } from './api.js'
export { AuthGate } from './AuthGate.js'
export { can } from './capabilities.js'
export { DOMAIN } from './domain.js'
export { catalogFor } from './languages.js'
export type { Catalog } from './languages.js'
export { isSessionRevoked } from './probe.js'
export { createAuthQueryClient } from './queryClient.js'
export { adoptSession, sessionQueryKey, useLogout, useSession } from './session.js'
export {
	SetupClosedError,
	SetupCodeError,
	SiteStartingError,
	checkSetupCode,
	createFirstAccount,
	isSetupOpen,
} from './setup.js'
export type { FirstAccount } from './setup.js'
export { configureAuthTransport, resetAuthTransport } from './transport.js'
export type { AuthTransport } from './transport.js'
