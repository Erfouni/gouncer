// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'

import { RateLimitedError } from '../api.js'
import { DOMAIN } from '../domain.js'
import { SetupClosedError, SetupCodeError, SiteStartingError } from '../setup.js'

/** The sentence each reason for a refused setup code shows. */
const codeWords: Record<SetupCodeError['reason'], () => string> = {
	invalid: () => __('Your setup code appears to be invalid. Please copy it again from the setup code file.', DOMAIN),
	expired: () => __('Your setup code has expired. A new one is waiting in the setup code file.', DOMAIN),
	file: () => __('The setup code file cannot be used. Please check it and try again.', DOMAIN),
}

/**
 * Returns the sentence for a refused setup code.
 * @param reason - Why the site refused the code.
 * @returns The sentence to show.
 */
export function codeRefusal(reason: SetupCodeError['reason']): string {
	return codeWords[reason]()
}

/**
 * Returns the sentence both setup steps show for a refusal they share.
 * @param error - The error the setup call threw.
 * @returns The sentence to show, or undefined for an error the steps do not share.
 */
export function setupRefusal(error: unknown): string | undefined {
	if (error instanceof SetupCodeError) {
		return codeRefusal(error.reason)
	}
	if (error instanceof SetupClosedError) {
		return __('You appear to have already set up this site.', DOMAIN)
	}
	if (error instanceof SiteStartingError) {
		return __('The site is starting. Please wait a moment and try again.', DOMAIN)
	}
	if (error instanceof RateLimitedError) {
		return __('Too many attempts. Please try again later.', DOMAIN)
	}
	return undefined
}
