// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { __, sprintf } from '@wordpress/i18n'
import { Button, Field, Input, Text } from '@wordpress/ui'
import { useId, useState } from 'react'
import type { ReactNode } from 'react'

import { DOMAIN } from '../domain.js'
import { SetupClosedError, checkSetupCode } from '../setup.js'
import type { SetupCodeError } from '../setup.js'
import { codeRefusal, setupRefusal } from './setupWords.js'

/**
 * Returns the sentence the code step shows, if any.
 * @param failure - The error the last check or hand-over raised, if any.
 * @param idle - Whether no check has run yet.
 * @param refused - The reason a later step sent the wizard back, if any.
 * @returns The sentence to show, or undefined for none.
 */
function stepMessage(
	failure: Error | null,
	idle: boolean,
	refused: SetupCodeError['reason'] | undefined,
): string | undefined {
	if (failure !== null) {
		return setupRefusal(failure) ?? __('The request failed, please try again.', DOMAIN)
	}
	if (idle && refused !== undefined) {
		return codeRefusal(refused)
	}
	return undefined
}

/**
 * Renders the step asking for the one-time setup code and hands the accepted code upward.
 * @param brand - The product name in the heading.
 * @param fileHint - The application's words for where its code file lives.
 * @param onUnlocked - Called with the code as typed once the site accepts it.
 * @param onClosed - Called when the site already has its first account.
 * @param refused - The reason a later step sent the wizard back, shown on arrival.
 * @returns The setup code step element.
 */
export function SetupCodeStep({
	brand,
	fileHint,
	onUnlocked,
	onClosed,
	refused,
}: {
	brand: string
	fileHint: ReactNode
	onUnlocked: (code: string) => void | Promise<void>
	onClosed?: () => void
	refused?: SetupCodeError['reason']
}) {
	const [code, setCode] = useState('')
	const [focusOnMount] = useState(() => window.matchMedia?.('(pointer: coarse)').matches !== true)
	const hintId = useId()
	const alertId = useId()
	const attempt = useMutation({
		mutationFn: async () => {
			await checkSetupCode(code)
			await onUnlocked(code)
		},
		onError: (error) => {
			if (error instanceof SetupClosedError) {
				onClosed?.()
			}
		},
	})
	const message = stepMessage(attempt.error, attempt.isIdle, refused)

	return (
		<form
			className="gopherium-setup"
			onSubmit={(event) => {
				event.preventDefault()
				attempt.mutate()
			}}
		>
			<Text variant="heading-lg" render={<h1 className="gopherium-setup__heading" />}>
				{sprintf(__('Unlock %s', DOMAIN), brand)}
			</Text>
			<Text render={<p className="gopherium-setup__text" />}>
				{__(
					'To ensure this site is securely set up by its owner, a setup code has been written to a file on the server.',
					DOMAIN,
				)}
			</Text>
			<Text render={<div id={hintId} className="gopherium-setup__file" />}>{fileHint}</Text>
			<Text render={<p className="gopherium-setup__text" />}>
				{__('Please copy the setup code from the file and paste it below.', DOMAIN)}
			</Text>
			{message === undefined ? null : (
				<Text role="alert" id={alertId} render={<p className="gopherium-setup__message" />}>
					{message}
				</Text>
			)}
			<Field.Root className="gopherium-setup__row" render={<div />}>
				<Field.Label className="gopherium-setup__label">{__('Setup code', DOMAIN)}</Field.Label>
				<div>
					<Input
						type="text"
						value={code}
						onChange={(event) => setCode(event.target.value)}
						autoComplete="off"
						spellCheck={false}
						autoFocus={focusOnMount}
						aria-describedby={message === undefined ? hintId : `${alertId} ${hintId}`}
					/>
				</div>
			</Field.Root>
			<p className="gopherium-setup__step">
				<Button type="submit" disabled={code.trim() === '' || attempt.isPending} loading={attempt.isPending}>
					{__('Continue', DOMAIN)}
				</Button>
			</p>
		</form>
	)
}
