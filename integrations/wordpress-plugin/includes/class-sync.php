<?php
/**
 * Wires post-lifecycle hooks to the MDDB client.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Translates WP post events into MDDB API calls.
 *
 * Hooks:
 *  - `wp_after_insert_post` → upsert when post is published (drafts skipped unless
 *    `includeDrafts` is on). Fires once per save, after relationships/meta land.
 *  - `before_delete_post`   → delete when post is hard-deleted.
 *  - `wp_trash_post`        → delete when post is trashed (treat trash as removal).
 */
class Sync {

	private Settings $settings;

	private Client $client;

	private Mapper $mapper;

	public function __construct( Settings $settings, Client $client, Mapper $mapper ) {
		$this->settings = $settings;
		$this->client   = $client;
		$this->mapper   = $mapper;
	}

	public function register(): void {
		add_action( 'wp_after_insert_post', [ $this, 'onSave' ], 20, 4 );
		add_action( 'before_delete_post', [ $this, 'onDelete' ], 10, 1 );
		add_action( 'wp_trash_post', [ $this, 'onDelete' ], 10, 1 );
	}

	/**
	 * @param int       $postId
	 * @param \WP_Post  $post
	 * @param bool      $update
	 * @param \WP_Post|null $postBefore
	 */
	public function onSave( int $postId, \WP_Post $post, bool $update, $postBefore ): void {
		unset( $update, $postBefore );

		if ( ! $this->settings->syncOnSave() || ! $this->settings->isConfigured() ) {
			return;
		}
		if ( wp_is_post_autosave( $postId ) || wp_is_post_revision( $postId ) ) {
			return;
		}
		if ( ! in_array( (string) $post->post_type, $this->settings->postTypes(), true ) ) {
			return;
		}
		if ( $post->post_status !== 'publish' && ! $this->settings->includeDrafts() ) {
			return;
		}
		// `trash`/`auto-draft` never sync via the save path — `onDelete` handles the trash flow.
		if ( in_array( (string) $post->post_status, [ 'trash', 'auto-draft', 'inherit' ], true ) ) {
			return;
		}
		if ( ! $this->matchesTermFilter( $post ) ) {
			return;
		}

		$document = $this->mapper->toDocument(
			$post,
			$this->settings->collection(),
			$this->settings->keyStrategy(),
			$this->settings->languageSource()
		);

		$result = $this->client->addDocument( $document );
		if ( is_wp_error( $result ) ) {
			$this->logError( 'mddb_sync_add', $result, $postId );
		}
	}

	/**
	 * AND-across-taxonomies, OR-within-taxonomy. An empty filter list for a
	 * given taxonomy imposes no constraint. Posts with no terms in a filtered
	 * taxonomy are excluded.
	 */
	public function matchesTermFilter( \WP_Post $post ): bool {
		$filter = $this->settings->termFilter();
		if ( count( $filter ) === 0 ) {
			return true;
		}
		foreach ( $filter as $taxonomy => $allowedIds ) {
			$terms = get_the_terms( $post, $taxonomy );
			if ( ! is_array( $terms ) || count( $terms ) === 0 ) {
				return false;
			}
			$postTermIds = [];
			foreach ( $terms as $term ) {
				if ( $term instanceof \WP_Term ) {
					$postTermIds[] = (int) $term->term_id;
				}
			}
			if ( count( array_intersect( $allowedIds, $postTermIds ) ) === 0 ) {
				return false;
			}
		}
		return true;
	}

	/**
	 * Sync hook used by the bulk-resync admin endpoint. Re-runs the same logic
	 * as `onSave` but without autosave/revision checks — the caller has already
	 * filtered to real posts via `WP_Query`.
	 *
	 * @return true|\WP_Error
	 */
	public function syncPost( \WP_Post $post ) {
		if ( ! $this->settings->isConfigured() ) {
			return new \WP_Error( 'mddb_sync_not_configured', 'MDDB URL is not configured.' );
		}
		if ( ! $this->matchesTermFilter( $post ) ) {
			return new \WP_Error( 'mddb_sync_term_filter_skip', 'Post excluded by term filter.' );
		}
		$document = $this->mapper->toDocument(
			$post,
			$this->settings->collection(),
			$this->settings->keyStrategy(),
			$this->settings->languageSource()
		);
		return $this->client->addDocument( $document );
	}

	public function onDelete( int $postId ): void {
		if ( ! $this->settings->syncOnDelete() || ! $this->settings->isConfigured() ) {
			return;
		}
		$post = get_post( $postId );
		if ( ! $post instanceof \WP_Post ) {
			return;
		}
		if ( ! in_array( (string) $post->post_type, $this->settings->postTypes(), true ) ) {
			return;
		}

		$key  = $this->mapper->keyFor( $post, $this->settings->keyStrategy() );
		$lang = ( new Language() )->detect( $postId, $this->settings->languageSource() );

		$result = $this->client->deleteDocument( $this->settings->collection(), $key, $lang );
		if ( is_wp_error( $result ) ) {
			$this->logError( 'mddb_sync_delete', $result, $postId );
		}
	}

	private function logError( string $tag, \WP_Error $error, int $postId ): void {
		if ( function_exists( 'error_log' ) ) {
			// phpcs:ignore WordPress.PHP.DevelopmentFunctions.error_log_error_log
			error_log(
				sprintf(
					'[%s] post=%d code=%s message=%s',
					$tag,
					$postId,
					$error->get_error_code(),
					$error->get_error_message()
				)
			);
		}
		/**
		 * Allow consumers (Action Scheduler, Sentry adapters, log shippers)
		 * to react to a sync failure without us pulling them in as deps.
		 */
		do_action( 'mddb_sync_error', $tag, $error, $postId );
	}
}
