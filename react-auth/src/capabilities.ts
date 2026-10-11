// SPDX-License-Identifier: Apache-2.0

import type { User } from './api.js'

/**
 * Reports whether the user holds the capability, a missing user holding none.
 * @param user - The signed-in user, if any.
 * @param capability - The capability asked for.
 * @returns Whether the user holds the capability.
 */
export function can(user: User | null | undefined, capability: string): boolean {
	return user?.capabilities?.includes(capability) ?? false
}
