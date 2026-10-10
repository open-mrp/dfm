package constants

// ObjectType is the value of a resource's "object" field.
type ObjectType string

const (
	ObjectTypeDFMCheck        ObjectType = "dfm_check"
	ObjectTypeDFMIssue        ObjectType = "dfm_issue"
	ObjectTypeDFMOccurrence   ObjectType = "dfm_occurrence"
	ObjectTypeDFMPart         ObjectType = "dfm_part"
	ObjectTypeDFMPartInstance ObjectType = "dfm_part_instance"
	ObjectTypeDFMHoleGroup    ObjectType = "dfm_hole_group"
	ObjectTypeDFMBend         ObjectType = "dfm_bend"
	ObjectTypeDFMSkippedCheck ObjectType = "dfm_skipped_check"
	ObjectTypeDFMFindingCode  ObjectType = "dfm_finding_code"
	ObjectTypeDFMPlan         ObjectType = "dfm_plan"
	ObjectTypeDFMOperation    ObjectType = "dfm_operation"
	ObjectTypeList            ObjectType = "list"
)

var objectTypes = []ObjectType{ObjectTypeDFMCheck, ObjectTypeDFMIssue, ObjectTypeDFMOccurrence, ObjectTypeDFMPart, ObjectTypeDFMPartInstance, ObjectTypeDFMHoleGroup, ObjectTypeDFMBend, ObjectTypeDFMSkippedCheck, ObjectTypeDFMFindingCode, ObjectTypeDFMPlan, ObjectTypeDFMOperation, ObjectTypeList}

func (o ObjectType) IsValid() bool        { return isOneOf(o, objectTypes) }
func (o ObjectType) EnumValues() []string { return enumValues(objectTypes) }
