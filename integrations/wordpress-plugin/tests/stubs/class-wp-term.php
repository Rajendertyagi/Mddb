<?php
/**
 * Minimal WP_Term stand-in for unit tests.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

/**
 * Public-property bag that lets mapper tests pass realistic objects to
 * `get_the_terms()` mocks.
 */
class WP_Term { // phpcs:ignore
	public int $term_id = 0; // phpcs:ignore
	public string $name = '';
	public string $slug = '';
	public string $taxonomy = '';
	public int $count = 0;

	/**
	 * @param array<string,mixed> $fields
	 */
	public function __construct( array $fields = [] ) {
		foreach ( $fields as $key => $value ) {
			if ( property_exists( $this, $key ) ) {
				$this->{$key} = $value;
			}
		}
	}
}
