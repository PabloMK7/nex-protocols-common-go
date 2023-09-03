package common_globals

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	nex "github.com/PretendoNetwork/nex-go"
	match_making "github.com/PretendoNetwork/nex-protocols-go/match-making"
	match_making_types "github.com/PretendoNetwork/nex-protocols-go/match-making/types"
	notifications "github.com/PretendoNetwork/nex-protocols-go/notifications"
	notifications_types "github.com/PretendoNetwork/nex-protocols-go/notifications/types"
	"golang.org/x/exp/slices"
)

// GetAvailableGatheringID returns a gathering ID which doesn't belong to any session
// Returns 0 if no IDs are available (math.MaxUint32 has been reached)
func GetAvailableGatheringID() uint32 {
	if CurrentGatheringID.Value() == math.MaxUint32 {
		return 0
	}

	return CurrentGatheringID.Increment()
}

// FindOtherConnectionID searches a connection ID on the session that isn't the given one
// Returns 0 if no connection ID could be found
func FindOtherConnectionID(excludedConnectionID uint32, gatheringID uint32) uint32 {
	for _, connectionID := range Sessions[gatheringID].ConnectionIDs {
		if connectionID != excludedConnectionID {
			return connectionID
		}
	}

	return 0
}

// RemoveConnectionIDFromSession removes a client from the session
func RemoveConnectionIDFromSession(clientConnectionID uint32, gathering uint32) {
	for index, connectionID := range Sessions[gathering].ConnectionIDs {
		if connectionID == clientConnectionID {
			Sessions[gathering].ConnectionIDs = DeleteIndex(Sessions[gathering].ConnectionIDs, index)
		}
	}

	if len(Sessions[gathering].ConnectionIDs) == 0 {
		delete(Sessions, gathering)
	}
}

// FindClientSession searches for session the given connection ID is connected to
func FindClientSession(connectionID uint32) uint32 {
	for gatheringID := range Sessions {
		if slices.Contains(Sessions[gatheringID].ConnectionIDs, connectionID) {
			return gatheringID
		}
	}

	return 0
}

// RemoveClientFromAllSessions removes a client from every session
func RemoveClientFromAllSessions(client *nex.Client) {
	// * Keep checking until no session is found
	for gid := FindClientSession(client.ConnectionID()); gid != 0; {
		session := Sessions[gid]
		lenParticipants := len(session.ConnectionIDs)

		RemoveConnectionIDFromSession(client.ConnectionID(), gid)

		if lenParticipants <= 1 {
			gid = FindClientSession(client.ConnectionID())
			continue
		}

		ownerPID := session.GameMatchmakeSession.Gathering.OwnerPID

		if client.PID() == ownerPID {
			// This flag tells the server to change the matchmake session owner if they disconnect
			// If the flag is not set, delete the session
			// More info: https://nintendo-wiki.pretendo.network/docs/nex/protocols/match-making/types#flags
			if session.GameMatchmakeSession.Gathering.Flags&match_making.GatheringFlags.DisconnectChangeOwner == 0 {
				delete(Sessions, gid)
			} else {
				ChangeSessionOwner(client, gid)
			}
		} else {
			server := client.Server()

			rmcMessage := nex.NewRMCRequest()
			rmcMessage.SetProtocolID(notifications.ProtocolID)
			rmcMessage.SetCallID(0xffff0000)
			rmcMessage.SetMethodID(notifications.MethodProcessNotificationEvent)

			category := notifications.NotificationCategories.Participation
			subtype := notifications.NotificationSubTypes.Participation.Disconnected

			oEvent := notifications_types.NewNotificationEvent()
			oEvent.PIDSource = client.PID()
			oEvent.Type = notifications.BuildNotificationType(category, subtype)
			oEvent.Param1 = gid
			oEvent.Param2 = client.PID()

			stream := nex.NewStreamOut(server)
			oEventBytes := oEvent.Bytes(stream)
			rmcMessage.SetParameters(oEventBytes)

			rmcMessageBytes := rmcMessage.Bytes()

			targetClient := server.FindClientFromPID(uint32(ownerPID))
			if targetClient == nil {
				// TODO - We don't have a logger here
				// logger.Warning("Owner client not found")
				gid = FindClientSession(client.ConnectionID())
				continue
			}

			var messagePacket nex.PacketInterface

			if server.PRUDPVersion() == 0 {
				messagePacket, _ = nex.NewPacketV0(targetClient, nil)
				messagePacket.SetVersion(0)
			} else {
				messagePacket, _ = nex.NewPacketV1(targetClient, nil)
				messagePacket.SetVersion(1)
			}
			messagePacket.SetSource(0xA1)
			messagePacket.SetDestination(0xAF)
			messagePacket.SetType(nex.DataPacket)
			messagePacket.SetPayload(rmcMessageBytes)

			messagePacket.AddFlag(nex.FlagNeedsAck)
			messagePacket.AddFlag(nex.FlagReliable)

			server.Send(messagePacket)
		}

		gid = FindClientSession(client.ConnectionID())
	}
}

// FindSessionByMatchmakeSession finds a gathering that matches with a MatchmakeSession
func FindSessionByMatchmakeSession(searchMatchmakeSession *match_making_types.MatchmakeSession) uint32 {
	// * This portion finds any sessions that match the search session
	// * It does not care about anything beyond that, such as if the match is already full
	// * This is handled below
	candidateSessionIndexes := make([]uint32, 0, len(Sessions))
	for index, session := range Sessions {
		if session.SearchMatchmakeSession.Equals(searchMatchmakeSession) {
			candidateSessionIndexes = append(candidateSessionIndexes, index)
		}
	}

	for _, sessionIndex := range candidateSessionIndexes {
		sessionToCheck := Sessions[sessionIndex]
		if len(sessionToCheck.ConnectionIDs) >= int(sessionToCheck.GameMatchmakeSession.MaximumParticipants) {
			continue
		}

		if !sessionToCheck.GameMatchmakeSession.OpenParticipation {
			continue
		}

		return sessionIndex // * Found a match
	}

	return 0
}

// FindSessionsByMatchmakeSessionSearchCriterias finds a gathering that matches with a MatchmakeSession
func FindSessionsByMatchmakeSessionSearchCriterias(lstSearchCriteria []*match_making_types.MatchmakeSessionSearchCriteria, gameSpecificChecks func(requestSearchCriteria, sessionSearchCriteria *match_making_types.MatchmakeSessionSearchCriteria) bool) []*CommonMatchmakeSession {
	// * This portion finds any sessions that match the search session
	// * It does not care about anything beyond that, such as if the match is already full
	// * This is handled below.
	candidateSessions := make([]*CommonMatchmakeSession, 0, len(Sessions))

	for _, session := range Sessions {
		if len(lstSearchCriteria) == len(session.SearchCriteria) {
			for criteriaIndex, sessionSearchCriteria := range session.SearchCriteria {
				requestSearchCriteria := lstSearchCriteria[criteriaIndex]

				// * Check things like game specific attributes
				if gameSpecificChecks != nil && !gameSpecificChecks(lstSearchCriteria[criteriaIndex], sessionSearchCriteria) {
					continue
				}

				if requestSearchCriteria.GameMode != "" && requestSearchCriteria.GameMode != sessionSearchCriteria.GameMode {
					continue
				}

				if requestSearchCriteria.MinParticipants != "" {
					split := strings.Split(requestSearchCriteria.MinParticipants, ",")
					minStr, maxStr := split[0], split[1]

					if minStr != "" {
						min, err := strconv.Atoi(minStr)
						if err != nil {
							// TODO - We don't have a logger here
							continue
						}

						if session.SearchMatchmakeSession.MinimumParticipants < uint16(min) {
							continue
						}
					}

					if maxStr != "" {
						max, err := strconv.Atoi(maxStr)
						if err != nil {
							// TODO - We don't have a logger here
							continue
						}

						if session.SearchMatchmakeSession.MinimumParticipants > uint16(max) {
							continue
						}
					}
				}

				if requestSearchCriteria.MaxParticipants != "" {
					split := strings.Split(requestSearchCriteria.MaxParticipants, ",")
					minStr := split[0]
					maxStr := ""

					if len(split) > 1 {
						maxStr = split[1]
					}

					if minStr != "" {
						min, err := strconv.Atoi(minStr)
						if err != nil {
							// TODO - We don't have a logger here
							continue
						}

						if session.SearchMatchmakeSession.MaximumParticipants < uint16(min) {
							continue
						}
					}

					if maxStr != "" {
						max, err := strconv.Atoi(maxStr)
						if err != nil {
							// TODO - We don't have a logger here
							continue
						}

						if session.SearchMatchmakeSession.MaximumParticipants > uint16(max) {
							continue
						}
					}
				}

				candidateSessions = append(candidateSessions, session)
			}
		}
	}

	filteredSessions := make([]*CommonMatchmakeSession, 0, len(candidateSessions))

	// * Further filter the candidate sessions
	for _, session := range candidateSessions {
		if len(session.ConnectionIDs) >= int(session.GameMatchmakeSession.MaximumParticipants) {
			continue
		}

		if !session.GameMatchmakeSession.OpenParticipation {
			continue
		}

		filteredSessions = append(filteredSessions, session) // * Found a match
	}

	return filteredSessions
}

// AddPlayersToSession updates the given sessions state to include the provided connection IDs
// Returns a NEX error code if failed
func AddPlayersToSession(session *CommonMatchmakeSession, connectionIDs []uint32) (error, uint32) {
	if (len(session.ConnectionIDs) + len(connectionIDs)) > int(session.GameMatchmakeSession.Gathering.MaximumParticipants) {
		return fmt.Errorf("Gathering %d is full", session.GameMatchmakeSession.Gathering.ID), nex.Errors.RendezVous.SessionFull
	}

	for _, connectedID := range connectionIDs {
		if slices.Contains(session.ConnectionIDs, connectedID) {
			return fmt.Errorf("Connection ID %d is already in gathering %d", connectedID, session.GameMatchmakeSession.Gathering.ID), nex.Errors.RendezVous.AlreadyParticipatedGathering
		}

		session.ConnectionIDs = append(session.ConnectionIDs, connectedID)

		session.GameMatchmakeSession.ParticipationCount += 1
	}

	return nil, 0
}

// ChangeSessionOwner changes the session owner to a different client
func ChangeSessionOwner(ownerClient *nex.Client, gathering uint32) {
	server := ownerClient.Server()
	var otherClient *nex.Client

	otherConnectionID := FindOtherConnectionID(ownerClient.ConnectionID(), gathering)
	if otherConnectionID != 0 {
		otherClient = server.FindClientFromConnectionID(uint32(otherConnectionID))
		if otherClient != nil {
			Sessions[gathering].GameMatchmakeSession.Gathering.OwnerPID = otherClient.PID()
		} else {
			// TODO - We don't have a logger here
			// logger.Warning("Other client not found")
			return
		}
	} else {
		return
	}

	rmcMessage := nex.NewRMCRequest()
	rmcMessage.SetProtocolID(notifications.ProtocolID)
	rmcMessage.SetCallID(0xffff0000)
	rmcMessage.SetMethodID(notifications.MethodProcessNotificationEvent)

	category := notifications.NotificationCategories.OwnershipChanged
	subtype := notifications.NotificationSubTypes.OwnershipChanged.None

	oEvent := notifications_types.NewNotificationEvent()
	oEvent.PIDSource = otherClient.PID()
	oEvent.Type = notifications.BuildNotificationType(category, subtype)
	oEvent.Param1 = gathering
	oEvent.Param2 = otherClient.PID()

	// TODO - StrParam doesn't have this value on some servers
	// https://github.com/kinnay/NintendoClients/issues/101
	// unixTime := time.Now()
	// oEvent.StrParam = strconv.FormatInt(unixTime.UnixMicro(), 10)

	stream := nex.NewStreamOut(server)
	oEventBytes := oEvent.Bytes(stream)
	rmcMessage.SetParameters(oEventBytes)

	rmcRequestBytes := rmcMessage.Bytes()

	for _, connectionID := range Sessions[gathering].ConnectionIDs {
		targetClient := server.FindClientFromConnectionID(connectionID)
		if targetClient != nil {
			var messagePacket nex.PacketInterface

			if server.PRUDPVersion() == 0 {
				messagePacket, _ = nex.NewPacketV0(targetClient, nil)
				messagePacket.SetVersion(0)
			} else {
				messagePacket, _ = nex.NewPacketV1(targetClient, nil)
				messagePacket.SetVersion(1)
			}

			messagePacket.SetSource(0xA1)
			messagePacket.SetDestination(0xAF)
			messagePacket.SetType(nex.DataPacket)
			messagePacket.SetPayload(rmcRequestBytes)

			messagePacket.AddFlag(nex.FlagNeedsAck)
			messagePacket.AddFlag(nex.FlagReliable)

			server.Send(messagePacket)
		} else {
			// TODO - We don't have a logger here
			// logger.Warning("Client not found")
		}
	}
}
