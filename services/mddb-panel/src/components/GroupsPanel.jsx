import { useEffect } from 'react';
import { Users, Clock } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import { formatDistanceToNow } from 'date-fns';

export default function GroupsPanel() {
  const {
    groups,
    groupsLoading,
    groupsError,
    setGroups,
    setGroupsLoading,
    setGroupsError,
  } = useStore();

  useEffect(() => {
    loadGroups();
  }, []);

  const loadGroups = async () => {
    setGroupsLoading(true);
    setGroupsError(null);
    try {
      const data = await mddbClient.listGroups();
      setGroups(data.groups || []);
    } catch (error) {
      setGroupsError(error.message);
      console.error('Failed to load groups:', error);
    } finally {
      setGroupsLoading(false);
    }
  };

  const formatDate = (timestamp) => {
    if (!timestamp) return 'N/A';
    const date = new Date(timestamp * 1000);
    return formatDistanceToNow(date, { addSuffix: true });
  };

  if (groupsLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  if (groupsError) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-red-600 font-medium mb-2">Failed to load groups</div>
          <div className="text-sm text-gray-500">{groupsError}</div>
          <button
            onClick={loadGroups}
            className="mt-4 px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-gray-50">
      <div className="bg-white border-b px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 mb-1">Group Management</h1>
            <p className="text-gray-600">Manage user groups and group permissions</p>
          </div>
          <button
            onClick={loadGroups}
            className="px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700"
          >
            Refresh
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="max-w-6xl mx-auto">
          {groups.length === 0 ? (
            <div className="bg-white rounded-lg shadow p-12 text-center">
              <Users className="w-16 h-16 mx-auto mb-4 text-gray-400" />
              <h3 className="text-lg font-medium text-gray-900 mb-2">No groups yet</h3>
              <p className="text-gray-600 mb-6">
                Create groups to organize users and manage permissions efficiently.
              </p>
            </div>
          ) : (
            <div className="grid gap-4">
              {groups.map((group) => (
                <div key={group.name} className="bg-white rounded-lg shadow p-6">
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex-1">
                      <h3 className="text-lg font-semibold text-gray-900 mb-1">{group.name}</h3>
                      {group.description && (
                        <p className="text-sm text-gray-600">{group.description}</p>
                      )}
                    </div>
                    <div className="flex items-center text-sm text-gray-500">
                      <Clock className="w-4 h-4 mr-1" />
                      {formatDate(group.createdAt)}
                    </div>
                  </div>

                  <div className="border-t pt-4">
                    <div className="flex items-center justify-between">
                      <div>
                        <span className="text-sm text-gray-600">Members:</span>
                        <span className="ml-2 text-sm font-medium text-gray-900">
                          {group.members?.length || 0}
                        </span>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {group.members && group.members.length > 0 ? (
                          group.members.slice(0, 5).map((member) => (
                            <span
                              key={member}
                              className="px-2 py-1 text-xs rounded-full bg-primary-100 text-primary-800"
                            >
                              {member}
                            </span>
                          ))
                        ) : (
                          <span className="text-sm text-gray-400">No members</span>
                        )}
                        {group.members && group.members.length > 5 && (
                          <span className="px-2 py-1 text-xs rounded-full bg-gray-100 text-gray-600">
                            +{group.members.length - 5} more
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Info Box */}
          <div className="mt-6 bg-blue-50 border border-blue-200 rounded-lg p-4">
            <p className="text-sm text-blue-800">
              <strong>Note:</strong> Group creation and member management are available via the API.
              Use the <code className="bg-blue-100 px-1 rounded">/v1/auth/groups</code> endpoint to create and manage groups,
              and <code className="bg-blue-100 px-1 rounded">/v1/auth/group-permissions</code> to set group permissions.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
