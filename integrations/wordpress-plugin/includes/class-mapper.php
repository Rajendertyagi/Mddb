<?php
/**
 * Converts a WP_Post to the MDDB /v1/add payload.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Pure mapping layer — no I/O, no hooks. Receives a `WP_Post` and emits
 * the JSON document we send to MDDB.
 *
 * v0.1.1 ships **full meta capture**:
 *   - All `get_post_meta()` keys (raw values, scalar/array-aware).
 *   - All taxonomies attached to the post type (not just category/post_tag).
 *   - ACF parsed values (`get_fields()`) layered on top — wins over raw keys.
 *   - Normalised SEO fields from Yoast / RankMath / SEOPress.
 *   - `mddb_sync_meta` filter for downstream customisation.
 *
 * Meta-key prefixes that ship as-is so consumers don't need to know which
 * plugin wrote what:
 *   - WordPress core (no prefix; e.g. `featured_video`).
 *   - Underscored "private" keys (`_yoast_wpseo_*`, `_thumbnail_id`, …).
 *   - Plugin keys (`rank_math_*`, `_seopress_*`, `_elementor_*`, …).
 *
 * MDDB schema is `map<string, []string>` — every value gets stringified.
 */
final class Mapper {

	private Language $language;

	private Seo $seo;

	public function __construct( Language $language, ?Seo $seo = null ) {
		$this->language = $language;
		$this->seo      = $seo ?? new Seo();
	}

	/**
	 * @return array{collection:string,key:string,lang:string,meta:array<string,array<int,string>>,contentMd:string}
	 */
	public function toDocument( \WP_Post $post, string $collection, string $keyStrategy, string $languageStrategy ): array {
		return [
			'collection' => $collection,
			'key'        => $this->keyFor( $post, $keyStrategy ),
			'lang'       => $this->language->detect( (int) $post->ID, $languageStrategy ),
			'meta'       => $this->metaFor( $post ),
			'contentMd'  => $this->bodyFor( $post ),
		];
	}

	public function keyFor( \WP_Post $post, string $strategy ): string {
		switch ( $strategy ) {
			case Settings::KEY_POST_SLUG:
				$slug = (string) $post->post_name;
				if ( $slug === '' ) {
					$slug = sanitize_title( (string) $post->post_title );
				}
				$prefix = sanitize_key( (string) $post->post_type );
				return $prefix . '-' . ( $slug !== '' ? $slug : (string) $post->ID );
			case Settings::KEY_PERMALINK:
				$permalink = get_permalink( $post );
				if ( is_string( $permalink ) && $permalink !== '' ) {
					$path = (string) wp_parse_url( $permalink, PHP_URL_PATH );
					$path = trim( $path, '/' );
					if ( $path !== '' ) {
						return $path;
					}
				}
				return sanitize_key( (string) $post->post_type ) . '-' . (string) $post->ID;
			case Settings::KEY_POST_ID:
			default:
				return sanitize_key( (string) $post->post_type ) . '-' . (string) $post->ID;
		}
	}

	/**
	 * @return array<string,array<int,string>>
	 */
	private function metaFor( \WP_Post $post ): array {
		$postId = (int) $post->ID;

		$meta = [
			'postType'    => [ (string) $post->post_type ],
			'status'      => [ (string) $post->post_status ],
			'title'       => [ (string) $post->post_title ],
			'slug'        => [ (string) $post->post_name ],
			'permalink'   => [ (string) get_permalink( $post ) ],
			'authorId'    => [ (string) $post->post_author ],
			'publishedAt' => [ (string) get_post_time( 'c', true, $post ) ],
			'modifiedAt'  => [ (string) get_post_modified_time( 'c', true, $post ) ],
		];

		$authorName = get_the_author_meta( 'display_name', (int) $post->post_author );
		if ( is_string( $authorName ) && $authorName !== '' ) {
			$meta['author'] = [ $authorName ];
		}

		$excerpt = (string) $post->post_excerpt;
		if ( $excerpt !== '' ) {
			$meta['excerpt'] = [ $excerpt ];
		}

		$meta = array_merge( $meta, $this->allTaxonomies( $post ) );
		$meta = array_merge( $meta, $this->postMeta( $postId ) );
		$meta = array_merge( $meta, $this->acfFields( $postId ) );
		$meta = array_merge( $meta, $this->seo->extract( $this->rawPostMeta( $postId ) ) );

		// Drop empty arrays / empty single values so MDDB meta stays compact.
		$meta = array_filter(
			$meta,
			static fn( array $values ): bool => count( $values ) > 0
				&& trim( implode( '', $values ) ) !== ''
		);

		/**
		 * Final say on the meta payload. Use to redact secrets, drop noisy keys
		 * (`_edit_lock`, `_edit_last`), or inject your own fields.
		 *
		 * @param array<string,array<int,string>> $meta
		 * @param \WP_Post                         $post
		 */
		$filtered = apply_filters( 'mddb_sync_meta', $meta, $post );
		return is_array( $filtered ) ? $filtered : $meta;
	}

	/**
	 * @return array<string,array<int,string>>
	 */
	private function allTaxonomies( \WP_Post $post ): array {
		if ( ! function_exists( 'get_object_taxonomies' ) ) {
			return [];
		}
		$taxonomies = get_object_taxonomies( (string) $post->post_type, 'names' );
		if ( ! is_array( $taxonomies ) ) {
			return [];
		}
		$out = [];
		foreach ( $taxonomies as $taxonomy ) {
			$names = $this->terms( $post, (string) $taxonomy );
			if ( count( $names ) > 0 ) {
				$key         = $this->taxonomyKey( (string) $taxonomy );
				$out[ $key ] = $names;
			}
		}
		return $out;
	}

	private function taxonomyKey( string $taxonomy ): string {
		// Legacy aliases preserved for backwards compatibility with consumers
		// of v0.1.0 documents.
		if ( $taxonomy === 'category' ) {
			return 'categories';
		}
		if ( $taxonomy === 'post_tag' ) {
			return 'tags';
		}
		return $taxonomy;
	}

	/**
	 * Raw `wp_postmeta` rows for the post, with every value coerced to string.
	 *
	 * @return array<string,array<int,string>>
	 */
	private function postMeta( int $postId ): array {
		$raw = $this->rawPostMeta( $postId );
		$out = [];
		foreach ( $raw as $key => $values ) {
			$strings = [];
			foreach ( $values as $value ) {
				$flat = $this->stringify( $value );
				if ( $flat !== '' ) {
					$strings[] = $flat;
				}
			}
			if ( count( $strings ) > 0 ) {
				$out[ (string) $key ] = $strings;
			}
		}
		return $out;
	}

	/**
	 * @return array<string,array<int,mixed>>
	 */
	private function rawPostMeta( int $postId ): array {
		if ( ! function_exists( 'get_post_meta' ) ) {
			return [];
		}
		$raw = get_post_meta( $postId );
		return is_array( $raw ) ? $raw : [];
	}

	/**
	 * Pulls **parsed** ACF values (`get_fields($postId)`). For an image field
	 * you get a URL/ID/array depending on the field's "Return format" instead
	 * of the raw attachment ID stored in `wp_postmeta`.
	 *
	 * Keys are namespaced with `acf:` so they don't collide with the raw
	 * `wp_postmeta` row of the same name.
	 *
	 * @return array<string,array<int,string>>
	 */
	private function acfFields( int $postId ): array {
		if ( ! function_exists( 'get_fields' ) ) {
			return [];
		}
		$fields = get_fields( $postId );
		if ( ! is_array( $fields ) ) {
			return [];
		}
		$out = [];
		foreach ( $fields as $name => $value ) {
			$flat = $this->stringify( $value );
			if ( $flat !== '' ) {
				$out[ 'acf:' . (string) $name ] = [ $flat ];
			}
		}
		return $out;
	}

	/**
	 * @return array<int,string>
	 */
	private function terms( \WP_Post $post, string $taxonomy ): array {
		$terms = get_the_terms( $post, $taxonomy );
		if ( ! is_array( $terms ) ) {
			return [];
		}
		$names = [];
		foreach ( $terms as $term ) {
			if ( $term instanceof \WP_Term && $term->name !== '' ) {
				$names[] = $term->name;
			}
		}
		return $names;
	}

	private function stringify( mixed $value ): string {
		if ( $value === null ) {
			return '';
		}
		if ( is_string( $value ) ) {
			return $value;
		}
		if ( is_bool( $value ) ) {
			return $value ? '1' : '0';
		}
		if ( is_scalar( $value ) ) {
			return (string) $value;
		}
		// WordPress hands back unserialized PHP arrays/objects for serialized
		// meta values. JSON-encode keeps them indexable while preserving shape.
		$encoded = wp_json_encode( $value );
		return is_string( $encoded ) ? $encoded : '';
	}

	private function bodyFor( \WP_Post $post ): string {
		$title  = (string) $post->post_title;
		// phpcs:ignore WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedHooknameFound -- core WP filter.
		$html   = (string) apply_filters( 'the_content', (string) $post->post_content );
		$plain  = wp_strip_all_tags( $html, true );
		$header = $title !== '' ? '# ' . $title . "\n\n" : '';
		return $header . trim( $plain ) . "\n";
	}
}
