// SPDX-License-Identifier: Apache-2.0

import { _x } from '@wordpress/i18n'

import { DOMAIN } from '../domain.js'

/** A meter answers a password's strength score, 0 to 4. */
export type Meter = (password: string, words: string[]) => number

/** The tone a strength shows in, named after the class it sets. */
export type Tone = 'short' | 'bad' | 'good' | 'strong'

/** The word and tone the meter shows for a score. */
export interface Level {
	word: string
	tone: Tone
}

/** The number of leading characters the meter scores. */
const SCORED_LENGTH = 64

/** The fewest characters a word needs to reach the meter. */
const SHORTEST_WORD = 4

/**
 * Returns the level for every score without one of its own, the lowest two included.
 * @returns The very weak level.
 */
function veryWeak(): Level {
	return { word: _x('Very weak', 'password strength', DOMAIN), tone: 'short' }
}

const levels = new Map<number, () => Level>([
	[-1, () => ({ word: _x('Password strength unknown', 'password strength', DOMAIN), tone: 'bad' })],
	[2, () => ({ word: _x('Weak', 'password strength', DOMAIN), tone: 'bad' })],
	[3, () => ({ word: _x('Medium', 'password strength', DOMAIN), tone: 'good' })],
	[4, () => ({ word: _x('Strong', 'password strength', DOMAIN), tone: 'strong' })],
])

/**
 * Loads zxcvbn on demand as its own chunk.
 * @returns The meter scoring a password against the given words.
 */
export async function loadMeter(): Promise<Meter> {
	const { default: zxcvbn } = await import('zxcvbn')
	return (password, words) => zxcvbn(password.slice(0, SCORED_LENGTH), words).score
}

/**
 * Splits values into the words the meter treats as easy to guess.
 * @param values - The values the person typed or the page shows.
 * @returns The distinct words of four characters or more.
 */
export function disallowedWords(values: string[]): string[] {
	const words = values.flatMap((value) => value.replace(/\W/g, ' ').split(' '))
	return words.filter((word, index) => word.length >= SHORTEST_WORD && words.indexOf(word) === index)
}

/**
 * Returns the word and tone the meter shows for a score.
 * @param score - The meter's score, or -1 while it is unknown.
 * @returns The word and tone.
 */
export function levelOf(score: number): Level {
	return (levels.get(score) ?? veryWeak)()
}
