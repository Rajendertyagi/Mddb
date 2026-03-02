import { useEffect } from 'react';
import { Users, Shield, Clock } from 'lucide-react';
import { useStore } from '../lib/store';
import mddbClient from '../lib/mddb-client';
import { formatDistanceToNow } from 'date-fns';

export default function UsersPanel() {
  const {
    users,
    usersLoading,
    usersError,
    setUsers,
    setUsersLoading,
    setUsersError,
  } = useStore();

  useEffect(() => {
    loadUsers();
  }, []);

  const loadUsers = async () => {
    setUsersLoading(true);
    setUsersError(null);
    try {
      const data = await mddbClient.listUsers();
      setUsers(data.users || []);
    } catch (error) {
      setUsersError(error.message);
      console.error('Failed to load users:', error);
    } finally {
      setUsersLoading(false);
    }
  };

  const formatDate = (timestamp) => {
    if (!timestamp) return 'N/A';
    const date = new Date(timestamp * 1000);
    return formatDistanceToNow(date, { addSuffix: true });
  };

  if (usersLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  if (usersError) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-center">
          <div className="text-red-600 font-medium mb-2">Failed to load users</div>
          <div className="text-sm text-gray-500">{usersError}</div>
          <button
            onClick={loadUsers}
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
            <h1 className="text-2xl font-bold text-gray-900 mb-1">User Management</h1>
            <p className="text-gray-600">Manage users and their permissions</p>
          </div>
          <button
            onClick={loadUsers}
            className="px-4 py-2 bg-primary-600 text-white rounded hover:bg-primary-700"
          >
            Refresh
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="max-w-6xl mx-auto">
          <div className="bg-white rounded-lg shadow overflow-hidden">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Username
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Created
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Role
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Groups
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Status
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {users.length === 0 ? (
                  <tr>
                    <td colSpan="5" className="px-6 py-12 text-center text-gray-500">
                      <Users className="w-12 h-12 mx-auto mb-2 text-gray-400" />
                      <div>No users found</div>
                    </td>
                  </tr>
                ) : (
                  users.map((user) => (
                    <tr key={user.username} className="hover:bg-gray-50">
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="flex items-center">
                          <div className="flex-shrink-0 h-8 w-8 bg-primary-100 rounded-full flex items-center justify-center">
                            <span className="text-primary-600 font-medium text-sm">
                              {user.username.charAt(0).toUpperCase()}
                            </span>
                          </div>
                          <div className="ml-3">
                            <div className="text-sm font-medium text-gray-900">{user.username}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="flex items-center text-sm text-gray-600">
                          <Clock className="w-4 h-4 mr-1" />
                          {formatDate(user.createdAt)}
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        {user.admin ? (
                          <span className="flex items-center px-2 py-1 text-xs font-medium rounded-full bg-purple-100 text-purple-800 w-fit">
                            <Shield className="w-3 h-3 mr-1" />
                            Admin
                          </span>
                        ) : (
                          <span className="px-2 py-1 text-xs font-medium rounded-full bg-gray-100 text-gray-800">
                            User
                          </span>
                        )}
                      </td>
                      <td className="px-6 py-4">
                        <div className="flex flex-wrap gap-1">
                          {user.groups && user.groups.length > 0 ? (
                            user.groups.map((group) => (
                              <span
                                key={group}
                                className="px-2 py-1 text-xs rounded-full bg-blue-100 text-blue-800"
                              >
                                {group}
                              </span>
                            ))
                          ) : (
                            <span className="text-sm text-gray-400">No groups</span>
                          )}
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        {user.disabled ? (
                          <span className="px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-800">
                            Disabled
                          </span>
                        ) : (
                          <span className="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-800">
                            Active
                          </span>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {/* Info Box */}
          <div className="mt-6 bg-blue-50 border border-blue-200 rounded-lg p-4">
            <p className="text-sm text-blue-800">
              <strong>Note:</strong> User creation and permission management are available via the API.
              Use the <code className="bg-blue-100 px-1 rounded">/v1/auth/register</code> endpoint to create new users
              and <code className="bg-blue-100 px-1 rounded">/v1/auth/permissions</code> to manage permissions.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
